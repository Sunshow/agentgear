import { create } from 'zustand'
import { useLogStore } from './log-store'

export interface ParamMapping {
  from: string
  to: string
  transform?: string
}

export interface HeaderInjection {
  key: string
  value: string
}

export interface ModelContextLimit {
  model_pattern: string
  token_limit: number
}

export interface ParamCondition {
  param: string
  op: 'prefix' | 'suffix' | 'contains' | 'equals'
  value: string
}

export interface TransformerDef {
  name: string
  description?: string
  direction: 'request' | 'response'
  type?: 'tool' | 'message_inject' | 'error_transform' | 'header_inject'
  
  // Tool transform fields
  source_tool?: string
  target_tool?: string
  accumulate: boolean
  param_mapping: ParamMapping[]
  input_schema?: Record<string, any>
  
  // Message inject fields
  inject_text?: string
  inject_format?: 'system-reminder' | 'plain'
  
  // Error transform fields
  error_code?: string
  error_message?: string
  request_size_threshold?: number
  context_token_limit?: number
  model_context_limits?: ModelContextLimit[]
  context_threshold_ratio?: number
  token_estimate_ratio?: number
  param_conditions?: ParamCondition[]
  
  // Common fields
  header_injections?: HeaderInjection[]
  builtin: boolean
  is_template?: boolean
  template_ref?: string
  template_args?: Record<string, string>
}

interface TransformerState {
  definitions: TransformerDef[]
  loading: boolean
  error: string | null
  editingDef: TransformerDef | null
  showForm: boolean
  selectedTemplate: TransformerDef | null

  fetchDefinitions: (apiUrl: string) => Promise<void>
  createDefinition: (apiUrl: string, def: TransformerDef) => Promise<boolean>
  updateDefinition: (apiUrl: string, name: string, def: TransformerDef) => Promise<boolean>
  deleteDefinition: (apiUrl: string, name: string) => Promise<boolean>
  setEditingDef: (def: TransformerDef | null) => void
  setShowForm: (show: boolean) => void
  setSelectedTemplate: (template: TransformerDef | null) => void
}

export const useTransformerStore = create<TransformerState>((set, get) => ({
  definitions: [],
  loading: false,
  error: null,
  editingDef: null,
  showForm: false,
  selectedTemplate: null,

  fetchDefinitions: async (apiUrl: string) => {
    set({ loading: true, error: null })
    const url = `${apiUrl}/api/transformers/defs`
    const startTime = Date.now()
    try {
      const res = await fetch(url)
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        response: { body: data },
      })
      set({ definitions: data.definitions || [], loading: false })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to fetch definitions', loading: false })
    }
  },

  createDefinition: async (apiUrl: string, def: TransformerDef) => {
    const url = `${apiUrl}/api/transformers/defs`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(def),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: def },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to create definition' })
        return false
      }
      await get().fetchDefinitions(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: 'error',
        duration,
        request: { body: def },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to create definition' })
      return false
    }
  },

  updateDefinition: async (apiUrl: string, name: string, def: TransformerDef) => {
    const url = `${apiUrl}/api/transformers/defs/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(def),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: def },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to update definition' })
        return false
      }
      await get().fetchDefinitions(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: 'error',
        duration,
        request: { body: def },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to update definition' })
      return false
    }
  },

  deleteDefinition: async (apiUrl: string, name: string) => {
    const url = `${apiUrl}/api/transformers/defs/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, { method: 'DELETE' })
      const duration = Date.now() - startTime
      let data = null
      if (!res.ok) {
        data = await res.json()
      }
      useLogStore.getState().addLog({
        method: 'DELETE',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        response: data ? { body: data } : undefined,
      })
      if (!res.ok) {
        set({ error: data?.error || 'Failed to delete definition' })
        return false
      }
      await get().fetchDefinitions(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'DELETE',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to delete definition' })
      return false
    }
  },

  setEditingDef: (def) => set({ editingDef: def }),
  setShowForm: (show) => set({ showForm: show, editingDef: show ? get().editingDef : null, selectedTemplate: show ? get().selectedTemplate : null }),
  setSelectedTemplate: (template) => set({ selectedTemplate: template }),
}))
