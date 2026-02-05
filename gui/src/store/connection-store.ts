import { create } from 'zustand'
import { useLogStore } from './log-store'

export interface ConnectionInfo {
  id: string
  session_id: string
  sequence: number
  tags: string[]
  method: string
  path: string
  status: string
  start_time: string
  end_time?: string
  duration_ms: number
  request_headers?: Record<string, string>
  request_body?: unknown
  request_tools?: { name: string; description?: string }[]
  response_status?: number
  response_headers?: Record<string, string>
  response_body?: unknown
  response_tools?: { id: string; name: string; input?: Record<string, unknown> }[]
  transformed_request: boolean
  transformed_response: boolean
  applied_transformers?: string[]
  parsed_data?: ParsedData
}

export interface ParsedData {
  protocol?: string
  anthropic?: AnthropicParsedData
}

export interface AnthropicParsedData {
  model?: string
  max_tokens?: number
  system_prompts?: SystemPrompt[]
  system_reminders?: SystemReminder[]
  tools?: ToolDefinition[]
}

export interface SystemPrompt {
  type: string
  text: string
  cache_control?: string
}

export interface SystemReminder {
  raw_text: string
  parsed_info?: Record<string, string>
}

export interface ToolDefinition {
  name: string
  description?: string
  input_schema?: Record<string, unknown>
}

interface ConnectionState {
  connections: ConnectionInfo[]
  selectedId: string | null
  isConnected: boolean
  apiUrl: string
  ws: WebSocket | null

  setApiUrl: (url: string) => void
  connect: () => void
  disconnect: () => void
  selectConnection: (id: string | null) => void
  fetchConnections: () => Promise<void>
  clearConnections: () => Promise<void>
}

export const useConnectionStore = create<ConnectionState>((set, get) => ({
  connections: [],
  selectedId: null,
  isConnected: false,
  apiUrl: 'http://localhost:9001',
  ws: null,

  setApiUrl: (url) => set({ apiUrl: url }),

  connect: async () => {
    const { apiUrl, fetchConnections } = get()

    try {
      await fetchConnections()

      const wsUrl = apiUrl.replace('http', 'ws') + '/api/ws'
      const ws = new WebSocket(wsUrl)

      ws.onopen = () => {
        set({ isConnected: true, ws })
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'connection') {
            const conn = msg.data as ConnectionInfo
            set((state) => {
              const existing = state.connections.findIndex((c) => c.id === conn.id)
              if (existing >= 0) {
                const updated = [...state.connections]
                updated[existing] = conn
                return { connections: updated }
              }
              return { connections: [conn, ...state.connections] }
            })
          }
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e)
        }
      }

      ws.onclose = () => {
        set({ isConnected: false, ws: null })
      }

      ws.onerror = () => {
        set({ isConnected: false, ws: null })
      }
    } catch (e) {
      console.error('Failed to connect:', e)
      set({ isConnected: false })
    }
  },

  disconnect: () => {
    const { ws } = get()
    if (ws) {
      ws.close()
    }
    set({ isConnected: false, ws: null, connections: [] })
  },

  selectConnection: (id) => set({ selectedId: id }),

  fetchConnections: async () => {
    const { apiUrl } = get()
    const url = `${apiUrl}/api/connections?limit=100`
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
      set({ connections: data.connections || [] })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'GET',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      console.error('Failed to fetch connections:', e)
    }
  },

  clearConnections: async () => {
    const { apiUrl } = get()
    const url = `${apiUrl}/api/connections`
    const startTime = Date.now()
    try {
      const res = await fetch(url, { method: 'DELETE' })
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'DELETE',
        url,
        status: res.ok ? 'success' : 'error',
        statusCode: res.status,
        duration,
      })
      set({ connections: [], selectedId: null })
    } catch (e) {
      const duration = Date.now() - startTime
      useLogStore.getState().addLog({
        method: 'DELETE',
        url,
        status: 'error',
        duration,
        error: e instanceof Error ? e.message : 'Unknown error',
      })
      console.error('Failed to clear connections:', e)
    }
  },
}))
