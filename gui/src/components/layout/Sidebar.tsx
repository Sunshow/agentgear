import { Settings, Activity, RefreshCw, Network, Tags, Shuffle, Link2 } from 'lucide-react'
import { useConnectionStore } from '../../store/connection-store'
import { cn } from '../../lib/utils'

interface SidebarProps {
  activeView: 'connections' | 'gateways' | 'tagging' | 'transformers' | 'mappings'
  onViewChange: (view: 'connections' | 'gateways' | 'tagging' | 'transformers' | 'mappings') => void
}

export function Sidebar({ activeView, onViewChange }: SidebarProps) {
  const { fetchConnections } = useConnectionStore()

  return (
    <div className="flex w-12 flex-col items-center border-r bg-muted/30 py-2">
      <button
        onClick={() => onViewChange('connections')}
        className={cn(
          'flex h-9 w-9 items-center justify-center rounded-md transition-colors',
          activeView === 'connections'
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
        )}
        title="Connections"
      >
        <Activity className="h-4 w-4" />
      </button>

      <button
        onClick={() => onViewChange('gateways')}
        className={cn(
          'mt-2 flex h-9 w-9 items-center justify-center rounded-md transition-colors',
          activeView === 'gateways'
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
        )}
        title="Gateways"
      >
        <Network className="h-4 w-4" />
      </button>

      <button
        onClick={() => onViewChange('tagging')}
        className={cn(
          'mt-2 flex h-9 w-9 items-center justify-center rounded-md transition-colors',
          activeView === 'tagging'
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
        )}
        title="Tagging Rules"
      >
        <Tags className="h-4 w-4" />
      </button>

      <button
        onClick={() => onViewChange('transformers')}
        className={cn(
          'mt-2 flex h-9 w-9 items-center justify-center rounded-md transition-colors',
          activeView === 'transformers'
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
        )}
        title="Transformers"
      >
        <Shuffle className="h-4 w-4" />
      </button>

      <button
        onClick={() => onViewChange('mappings')}
        className={cn(
          'mt-2 flex h-9 w-9 items-center justify-center rounded-md transition-colors',
          activeView === 'mappings'
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
        )}
        title="Mappings"
      >
        <Link2 className="h-4 w-4" />
      </button>

      <div className="my-4 h-px w-6 bg-border" />

      <button
        onClick={() => fetchConnections()}
        className="flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        title="Refresh"
      >
        <RefreshCw className="h-4 w-4" />
      </button>

      <div className="flex-1" />

      <button
        className="flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        title="Settings"
      >
        <Settings className="h-4 w-4" />
      </button>
    </div>
  )
}
