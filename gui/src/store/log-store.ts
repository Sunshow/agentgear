import { create } from 'zustand'

export interface LogEntry {
  id: string
  timestamp: Date
  method: string
  url: string
  status: 'success' | 'error'
  statusCode?: number
  duration?: number
  request?: { headers?: Record<string, string>; body?: unknown }
  response?: { headers?: Record<string, string>; body?: unknown }
  error?: string
}

interface LogState {
  logs: LogEntry[]
  unreadCount: number
  isOpen: boolean
  addLog: (log: Omit<LogEntry, 'id' | 'timestamp'>) => void
  markAsRead: () => void
  setOpen: (open: boolean) => void
  clearLogs: () => void
}

export const useLogStore = create<LogState>((set, get) => ({
  logs: [],
  unreadCount: 0,
  isOpen: false,

  addLog: (log) => {
    const entry: LogEntry = {
      ...log,
      id: crypto.randomUUID(),
      timestamp: new Date(),
    }
    set((state) => ({
      logs: [entry, ...state.logs].slice(0, 200),
      unreadCount: state.isOpen ? state.unreadCount : state.unreadCount + 1,
    }))
  },

  markAsRead: () => set({ unreadCount: 0 }),

  setOpen: (open) => {
    set({ isOpen: open })
    if (open) {
      get().markAsRead()
    }
  },

  clearLogs: () => set({ logs: [], unreadCount: 0 }),
}))
