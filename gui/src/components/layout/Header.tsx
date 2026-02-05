import { Plug, RotateCw, Save, ScrollText, Sun, Moon, Monitor } from 'lucide-react'
import { toast } from 'sonner'
import { useConnectionStore } from '../../store/connection-store'
import { useGatewayStore } from '../../store/gateway-store'
import { useLogStore } from '../../store/log-store'
import { useThemeStore } from '../../store/theme-store'
import { Sheet } from '../ui/sheet'
import { LogPanel } from '../logs/LogPanel'
import { cn } from '../../lib/utils'

export function Header() {
  const { isConnected, connect, disconnect, apiUrl } = useConnectionStore()
  const { restartProxy, restarting, saveConfig, saving } = useGatewayStore()
  const { unreadCount, isOpen, setOpen } = useLogStore()
  const { theme, setTheme } = useThemeStore()

  const handleRestart = () => {
    restartProxy(apiUrl)
  }

  const handleSave = async () => {
    const success = await saveConfig(apiUrl)
    if (success) {
      toast.success('Config saved successfully')
    } else {
      toast.error('Failed to save config')
    }
  }

  const cycleTheme = () => {
    const next = theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system'
    setTheme(next)
  }

  const ThemeIcon = theme === 'system' ? Monitor : theme === 'light' ? Sun : Moon

  return (
    <>
      <div className="flex h-10 items-center justify-between border-b px-4">
        <h1 className="text-sm font-semibold">AgentGear</h1>
        <div className="flex items-center gap-2">
          <button
            onClick={cycleTheme}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-md border transition-colors',
              'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            )}
            title={`Theme: ${theme}`}
          >
            <ThemeIcon className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => setOpen(true)}
            className={cn(
              'relative flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
              'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            )}
          >
            <ScrollText className="h-3.5 w-3.5" />
            Logs
            {unreadCount > 0 && (
              <span className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
                {unreadCount > 9 ? '9+' : unreadCount}
              </span>
            )}
          </button>
          <button
            onClick={() => (isConnected ? disconnect() : connect())}
            className={cn(
              'flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
              isConnected
                ? 'border-green-500/30 bg-green-500/10 text-green-500 hover:bg-green-500/20'
                : 'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            )}
          >
            <Plug className="h-3.5 w-3.5" />
            {isConnected ? 'Connected' : 'Connect'}
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className={cn(
              'flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
              'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              'disabled:cursor-not-allowed disabled:opacity-50'
            )}
          >
            <Save className="h-3.5 w-3.5" />
            {saving ? 'Saving...' : 'Save'}
          </button>
          <button
            onClick={handleRestart}
            disabled={restarting}
            className={cn(
              'flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-colors',
              'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground',
              'disabled:cursor-not-allowed disabled:opacity-50'
            )}
          >
            <RotateCw className={cn('h-3.5 w-3.5', restarting && 'animate-spin')} />
            {restarting ? 'Restarting...' : 'Restart'}
          </button>
        </div>
      </div>
      <Sheet open={isOpen} onClose={() => setOpen(false)} title="Proxy Logs" width="w-[480px]">
        <LogPanel />
      </Sheet>
    </>
  )
}
