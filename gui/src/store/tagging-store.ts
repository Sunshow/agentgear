import { create } from 'zustand'
import { useLogStore } from './log-store'

export type MatchOp = 'exists' | 'not_exists' | 'eq' | 'ne' | 'contains' | 'not_contains' | 'prefix' | 'suffix' | 'regex' | 'in' | 'not_in'
export type MatcherType = 'header' | 'tag' | 'tags'
export type TagMatchOp = 'all' | 'any'

export interface ValueMatcher {
  op: MatchOp
  value?: string
  values?: string[]
}

export interface Matcher {
  type: MatcherType
  key?: string
  match?: ValueMatcher
  tag?: string
  tags?: string[]
  tag_op?: TagMatchOp
}

export interface TaggingRule {
  name: string
  priority: number
  enabled: boolean
  builtin: boolean
  matchers: Matcher[]
  tags: string[]
}

export interface TagWithCount {
  name: string
  count: number
}

interface TaggingState {
  rules: TaggingRule[]
  actualTags: TagWithCount[]
  loading: boolean
  error: string | null
  editingRule: TaggingRule | null
  showForm: boolean

  fetchRules: (apiUrl: string) => Promise<void>
  fetchActualTags: (apiUrl: string) => Promise<void>
  createRule: (apiUrl: string, rule: TaggingRule) => Promise<boolean>
  updateRule: (apiUrl: string, name: string, rule: TaggingRule) => Promise<boolean>
  deleteRule: (apiUrl: string, name: string) => Promise<boolean>
  setEditingRule: (rule: TaggingRule | null) => void
  setShowForm: (show: boolean) => void
}

export const useTaggingStore = create<TaggingState>((set, get) => ({
  rules: [],
  actualTags: [],
  loading: false,
  error: null,
  editingRule: null,
  showForm: false,

  fetchRules: async (apiUrl: string) => {
    set({ loading: true, error: null })
    const url = `${apiUrl}/api/tagging/rules`
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
      set({ rules: data.rules || [], loading: false })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to fetch rules', loading: false })
    }
  },

  fetchActualTags: async (apiUrl: string) => {
    const url = `${apiUrl}/api/tags`
    try {
      const res = await fetch(url)
      const data = await res.json()
      set({ actualTags: data.tags || [] })
    } catch (e) {
      console.error('Failed to fetch actual tags:', e)
    }
  },

  createRule: async (apiUrl: string, rule: TaggingRule) => {
    const url = `${apiUrl}/api/tagging/rules/${encodeURIComponent(rule.name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rule),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: rule },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to create rule' })
        return false
      }
      await get().fetchRules(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: 'error',
        duration,
        request: { body: rule },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to create rule' })
      return false
    }
  },

  updateRule: async (apiUrl: string, name: string, rule: TaggingRule) => {
    const url = `${apiUrl}/api/tagging/rules/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rule),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: rule },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to update rule' })
        return false
      }
      await get().fetchRules(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: 'error',
        duration,
        request: { body: rule },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to update rule' })
      return false
    }
  },

  deleteRule: async (apiUrl: string, name: string) => {
    const url = `${apiUrl}/api/tagging/rules/${encodeURIComponent(name)}`
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
        set({ error: data?.error || 'Failed to delete rule' })
        return false
      }
      await get().fetchRules(apiUrl)
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
      set({ error: 'Failed to delete rule' })
      return false
    }
  },

  setEditingRule: (rule) => set({ editingRule: rule }),
  setShowForm: (show) => set({ showForm: show, editingRule: show ? get().editingRule : null }),
}))
