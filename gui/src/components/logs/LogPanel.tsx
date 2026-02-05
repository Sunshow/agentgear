import { useState } from 'react'
import { Trash2, ChevronDown, ChevronRight, CheckCircle, XCircle } from 'lucide-react'
import { useLogStore, LogEntry } from '../../store/log-store'
import { cn } from '../../lib/utils'

function LogItem({ log }: { log: LogEntry }) {
  const [expanded, setExpanded] = useState(false)

  const formatTime = (date: Date) => {
    return date.toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  return (
    <div className="border-b">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-accent/50"
      >
        {expanded ? (
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        {log.status === 'success' ? (
          <CheckCircle className="h-3.5 w-3.5 shrink-0 text-green-500" />
        ) : (
          <XCircle className="h-3.5 w-3.5 shrink-0 text-red-500" />
        )}
        <span className="shrink-0 text-xs text-muted-foreground">{formatTime(log.timestamp)}</span>
        <span
          className={cn(
            'shrink-0 rounded px-1.5 py-0.5 text-xs font-medium',
            log.method === 'GET' && 'bg-blue-500/10 text-blue-500',
            log.method === 'POST' && 'bg-green-500/10 text-green-500',
            log.method === 'PUT' && 'bg-yellow-500/10 text-yellow-500',
            log.method === 'DELETE' && 'bg-red-500/10 text-red-500'
          )}
        >
          {log.method}
        </span>
        <span className="truncate text-xs">{log.url}</span>
        {log.duration !== undefined && (
          <span className="ml-auto shrink-0 text-xs text-muted-foreground">{log.duration}ms</span>
        )}
      </button>
      {expanded && (
        <div className="bg-muted/30 px-3 py-2 text-xs">
          {log.statusCode && (
            <div className="mb-2">
              <span className="font-medium">Status: </span>
              <span className={log.status === 'success' ? 'text-green-500' : 'text-red-500'}>
                {log.statusCode}
              </span>
            </div>
          )}
          {log.error && (
            <div className="mb-2">
              <span className="font-medium">Error: </span>
              <span className="text-red-500">{log.error}</span>
            </div>
          )}
          {log.request?.body !== undefined && (
            <div className="mb-2">
              <div className="mb-1 font-medium">Request Body:</div>
              <pre className="max-h-32 overflow-auto rounded bg-background p-2 text-[10px]">
                {JSON.stringify(log.request.body, null, 2) ?? ''}
              </pre>
            </div>
          )}
          {log.response?.body !== undefined && (
            <div>
              <div className="mb-1 font-medium">Response Body:</div>
              <pre className="max-h-32 overflow-auto rounded bg-background p-2 text-[10px]">
                {JSON.stringify(log.response.body, null, 2) ?? ''}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export function LogPanel() {
  const { logs, clearLogs } = useLogStore()

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <span className="text-xs text-muted-foreground">{logs.length} logs</span>
        <button
          onClick={clearLogs}
          disabled={logs.length === 0}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
        >
          <Trash2 className="h-3 w-3" />
          Clear
        </button>
      </div>
      <div className="flex-1 overflow-auto">
        {logs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            No logs yet
          </div>
        ) : (
          logs.map((log) => <LogItem key={log.id} log={log} />)
        )}
      </div>
    </div>
  )
}
