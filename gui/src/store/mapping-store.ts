import { create } from 'zustand'
import { useLogStore } from './log-store'

export interface MappingRule {
  name: string
  description?: string
  enabled: boolean
  tags: string[]
  exclude_tags?: string[]
  gateways: string[]
  tools?: string[]
  tool_op?: string
  transformer: string
  builtin: boolean
}

interface MappingState {
  mappings: MappingRule[]
  loading: boolean
  error: string | null
  editingMapping: MappingRule | null
  showForm: boolean
  prefilledTransformer: string | null

  fetchMappings: (apiUrl: string) => Promise<void>
  createMapping: (apiUrl: string, mapping: MappingRule) => Promise<boolean>
  updateMapping: (apiUrl: string, name: string, mapping: MappingRule) => Promise<boolean>
  deleteMapping: (apiUrl: string, name: string) => Promise<boolean>
  setEditingMapping: (mapping: MappingRule | null) => void
  setShowForm: (show: boolean, prefilledTransformer?: string | null) => void
}

export const useMappingStore = create<MappingState>((set, get) => ({
  mappings: [],
  loading: false,
  error: null,
  editingMapping: null,
  showForm: false,
  prefilledTransformer: null,

  fetchMappings: async (apiUrl: string) => {
    set({ loading: true, error: null })
    const url = `${apiUrl}/api/mappings`
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
      set({ mappings: data.mappings || [], loading: false })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to fetch mappings', loading: false })
    }
  },

  createMapping: async (apiUrl: string, mapping: MappingRule) => {
    const url = `${apiUrl}/api/mappings`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(mapping),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: mapping },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to create mapping' })
        return false
      }
      await get().fetchMappings(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: 'error',
        duration,
        request: { body: mapping },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to create mapping' })
      return false
    }
  },

  updateMapping: async (apiUrl: string, name: string, mapping: MappingRule) => {
    const url = `${apiUrl}/api/mappings/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(mapping),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: mapping },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to update mapping' })
        return false
      }
      await get().fetchMappings(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: 'error',
        duration,
        request: { body: mapping },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to update mapping' })
      return false
    }
  },

  deleteMapping: async (apiUrl: string, name: string) => {
    const url = `${apiUrl}/api/mappings/${encodeURIComponent(name)}`
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
        set({ error: data?.error || 'Failed to delete mapping' })
        return false
      }
      await get().fetchMappings(apiUrl)
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
      set({ error: 'Failed to delete mapping' })
      return false
    }
  },

  setEditingMapping: (mapping) => set({ editingMapping: mapping }),
  setShowForm: (show, prefilledTransformer = null) =>
    set({
      showForm: show,
      editingMapping: show ? get().editingMapping : null,
      prefilledTransformer: show ? prefilledTransformer : null,
    }),
}))
