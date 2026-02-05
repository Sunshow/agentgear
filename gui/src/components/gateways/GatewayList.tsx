import { useEffect } from 'react'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { useGatewayStore, GatewayConfig } from '../../store/gateway-store'
import { useConnectionStore } from '../../store/connection-store'
import { GatewayForm } from './GatewayForm'
import { PageContainer } from '../layout/PageContainer'
import { cn } from '../../lib/utils'

export function GatewayList() {
  const { gateways, loading, fetchGateways, deleteGateway, showForm, setShowForm, editingGateway, setEditingGateway } = useGatewayStore()
  const { apiUrl } = useConnectionStore()

  useEffect(() => {
    fetchGateways(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const handleAdd = () => {
    setEditingGateway(null)
    setShowForm(true)
  }

  const handleEdit = (gw: GatewayConfig) => {
    setEditingGateway(gw)
    setShowForm(true)
  }

  const handleDelete = async (name: string) => {
    if (confirm(`Are you sure you want to delete gateway "${name}"?`)) {
      await deleteGateway(apiUrl, name)
    }
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingGateway(null)
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        Loading...
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b px-4 py-2">
        <h2 className="text-sm font-semibold">Gateways</h2>
        <button
          onClick={handleAdd}
          className="flex h-7 items-center gap-1 rounded-md border bg-background px-2 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Plus className="h-3.5 w-3.5" />
          Add
        </button>
      </div>

      <PageContainer className="p-4">
        {gateways.length === 0 ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            No gateways configured
          </div>
        ) : (
          <div className="space-y-3">
            {gateways.map((gw) => (
              <div
                key={gw.name}
                className={cn(
                  'rounded-lg border p-4',
                  gw.enabled ? 'border-green-500/30 bg-green-500/5' : 'border-muted bg-muted/30'
                )}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{gw.name}</span>
                    <span
                      className={cn(
                        'rounded px-1.5 py-0.5 text-xs',
                        gw.enabled
                          ? 'bg-green-500/20 text-green-500'
                          : 'bg-muted text-muted-foreground'
                      )}
                    >
                      {gw.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </div>
                  <div className="flex items-center gap-1">
                    <span className="mr-2 text-xs text-muted-foreground">{gw.timeout}s</span>
                    <button
                      onClick={() => handleEdit(gw)}
                      className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                      title="Edit"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => handleDelete(gw.name)}
                      className="rounded p-1.5 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                      title="Delete"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-2 space-y-1 text-sm">
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground">Path:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{gw.path}/*</code>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground">Upstream:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{gw.upstream}</code>
                  </div>
                  {gw.type && (
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground">Type:</span>
                      <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{gw.type}</code>
                      <span className="text-xs text-muted-foreground">→ $u_{gw.type}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </PageContainer>

      <GatewayForm open={showForm} onClose={handleCloseForm} gateway={editingGateway} />
    </div>
  )
}
