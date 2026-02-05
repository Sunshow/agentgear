import { Trash2 } from 'lucide-react'
import { useConnectionStore } from '../../store/connection-store'
import { cn } from '../../lib/utils'
import { ScrollArea } from '../ui/scroll-area'
import { TagGroupCompact } from '../ui/tag-group'
import { Button } from '../ui/dialog'

export function ConnectionList() {
  const { connections, selectedId, selectConnection, isConnected, clearConnections } = useConnectionStore()

  if (!isConnected) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-4 text-center text-muted-foreground">
        <p className="text-sm">Not connected to proxy</p>
        <p className="mt-1 text-xs">Click the plug icon to connect</p>
      </div>
    )
  }

  if (connections.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-4 text-center text-muted-foreground">
        <p className="text-sm">No connections yet</p>
        <p className="mt-1 text-xs">Connections will appear here</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b p-2">
        <span className="text-xs text-muted-foreground">{connections.length} connections</span>
        <Button
          variant="outline"
          className="h-7 px-2"
          onClick={clearConnections}
          title="Clear all connections"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-1 p-2">
        {connections.map((conn) => (
          <button
            key={conn.id}
            onClick={() => selectConnection(conn.id)}
            className={cn(
              'flex flex-col items-start rounded-md border p-2 text-left transition-colors',
              selectedId === conn.id
                ? 'border-primary bg-primary/5'
                : 'border-transparent hover:bg-accent'
            )}
          >
            <div className="flex w-full items-center justify-between">
              <span className="text-xs font-medium">
                {conn.method} {conn.path}
              </span>
              <StatusBadge status={conn.status} />
            </div>

            <div className="mt-1 flex w-full items-center gap-2 text-xs text-muted-foreground">
              <span>{conn.session_id?.slice(-8)}</span>
              <span>#{conn.sequence}</span>
              {conn.duration_ms > 0 && <span>{conn.duration_ms}ms</span>}
            </div>

            {conn.tags && conn.tags.length > 0 && (
              <div className="mt-1">
                <TagGroupCompact tags={conn.tags} maxTags={3} size="xs" />
              </div>
            )}
          </button>
        ))}
        </div>
      </ScrollArea>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={cn(
        'rounded px-1.5 py-0.5 text-[10px] font-medium',
        status === 'completed' && 'bg-green-500/20 text-green-600',
        status === 'pending' && 'bg-yellow-500/20 text-yellow-600',
        status === 'error' && 'bg-red-500/20 text-red-600'
      )}
    >
      {status}
    </span>
  )
}
