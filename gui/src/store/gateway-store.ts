import { create } from 'zustand'
import { useLogStore } from './log-store'

export interface GatewayConfig {
  name: string
  path: string
  upstream: string
  type: string
  timeout: number
  enabled: boolean
}

interface GatewayState {
  gateways: GatewayConfig[]
  loading: boolean
  error: string | null
  restarting: boolean
  saving: boolean
  editingGateway: GatewayConfig | null
  showForm: boolean

  fetchGateways: (apiUrl: string) => Promise<void>
  createGateway: (apiUrl: string, gw: GatewayConfig) => Promise<boolean>
  updateGateway: (apiUrl: string, name: string, gw: GatewayConfig) => Promise<boolean>
  deleteGateway: (apiUrl: string, name: string) => Promise<boolean>
  restartProxy: (apiUrl: string) => Promise<void>
  saveConfig: (apiUrl: string) => Promise<boolean>
  setEditingGateway: (gw: GatewayConfig | null) => void
  setShowForm: (show: boolean) => void
}

export const useGatewayStore = create<GatewayState>((set, get) => ({
  gateways: [],
  loading: false,
  error: null,
  restarting: false,
  saving: false,
  editingGateway: null,
  showForm: false,

  fetchGateways: async (apiUrl: string) => {
    set({ loading: true, error: null })
    const url = `${apiUrl}/api/gateways`
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
      set({ gateways: data.gateways || [], loading: false })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to fetch gateways', loading: false })
      console.error('Failed to fetch gateways:', e)
    }
  },

  createGateway: async (apiUrl: string, gw: GatewayConfig) => {
    const url = `${apiUrl}/api/gateways`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(gw),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: gw },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to create gateway' })
        return false
      }
      await get().fetchGateways(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: 'error',
        duration,
        request: { body: gw },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to create gateway' })
      console.error('Failed to create gateway:', e)
      return false
    }
  },

  updateGateway: async (apiUrl: string, name: string, gw: GatewayConfig) => {
    const url = `${apiUrl}/api/gateways/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(gw),
      })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        request: { body: gw },
        response: { body: data },
      })
      if (!res.ok) {
        set({ error: data.error || 'Failed to update gateway' })
        return false
      }
      await get().fetchGateways(apiUrl)
      return true
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'PUT',
        url,
        status: 'error',
        duration,
        request: { body: gw },
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      set({ error: 'Failed to update gateway' })
      console.error('Failed to update gateway:', e)
      return false
    }
  },

  deleteGateway: async (apiUrl: string, name: string) => {
    const url = `${apiUrl}/api/gateways/${encodeURIComponent(name)}`
    const startTime = Date.now()
    try {
      const res = await fetch(url, {
        method: 'DELETE',
      })
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
        set({ error: data?.error || 'Failed to delete gateway' })
        return false
      }
      await get().fetchGateways(apiUrl)
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
      set({ error: 'Failed to delete gateway' })
      console.error('Failed to delete gateway:', e)
      return false
    }
  },

  restartProxy: async (apiUrl: string) => {
    set({ restarting: true })
    const url = `${apiUrl}/api/restart`
    const startTime = Date.now()
    try {
      const res = await fetch(url, { method: 'POST' })
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
      })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      console.error('Failed to restart proxy:', e)
    }
    setTimeout(() => {
      set({ restarting: false })
    }, 3000)
  },

  saveConfig: async (apiUrl: string) => {
    set({ saving: true })
    const url = `${apiUrl}/api/config/save`
    const startTime = Date.now()
    try {
      const res = await fetch(url, { method: 'POST' })
      const data = await res.json()
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
        response: { body: data },
      })
      set({ saving: false })
      return res.ok
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'POST',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      console.error('Failed to save config:', e)
      set({ saving: false })
      return false
    }
  },

  setEditingGateway: (gw) => set({ editingGateway: gw }),
  setShowForm: (show) => set({ showForm: show, editingGateway: show ? get().editingGateway : null }),
}))
