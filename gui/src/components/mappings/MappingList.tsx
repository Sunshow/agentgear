import { useEffect, useMemo } from 'react'
import { Plus, Pencil, Trash2, Lock } from 'lucide-react'
import { useMappingStore, MappingRule } from '../../store/mapping-store'
import { useTransformerStore } from '../../store/transformer-store'
import { useConnectionStore } from '../../store/connection-store'
import { MappingForm } from './MappingForm'
import { PageContainer } from '../layout/PageContainer'
import { cn } from '../../lib/utils'

export function MappingList() {
  const { mappings, loading, fetchMappings, deleteMapping, showForm, setShowForm, setEditingMapping } =
    useMappingStore()
  const { definitions, fetchDefinitions } = useTransformerStore()
  const { apiUrl } = useConnectionStore()

  useEffect(() => {
    fetchMappings(apiUrl)
    fetchDefinitions(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const { responseMappings, requestMappings } = useMemo(() => {
    const getDirection = (transformerName: string) => {
      const def = definitions.find((d) => d.name === transformerName)
      return def?.direction
    }
    return {
      responseMappings: mappings.filter((m) => getDirection(m.transformer) === 'response'),
      requestMappings: mappings.filter((m) => getDirection(m.transformer) === 'request'),
    }
  }, [mappings, definitions])

  const handleAdd = () => {
    setEditingMapping(null)
    setShowForm(true)
  }

  const handleEdit = (mapping: MappingRule) => {
    setEditingMapping(mapping)
    setShowForm(true)
  }

  const handleDelete = async (name: string) => {
    if (confirm(`Are you sure you want to delete mapping "${name}"?`)) {
      await deleteMapping(apiUrl, name)
    }
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingMapping(null)
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
        <h2 className="text-sm font-semibold">Mappings</h2>
        <button
          onClick={handleAdd}
          className="flex h-7 items-center gap-1 rounded-md border bg-background px-2 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Plus className="h-3.5 w-3.5" />
          Add
        </button>
      </div>

      <PageContainer className="p-4">
        {mappings.length === 0 ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            No mappings configured
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-4">
            {/* 左侧: Response */}
            <div>
              <h3 className="mb-2 text-xs font-medium uppercase text-purple-500">
                Response ({responseMappings.length})
              </h3>
              <div className="space-y-2">
                {responseMappings.length === 0 ? (
                  <p className="text-xs text-muted-foreground">No response mappings</p>
                ) : (
                  responseMappings.map((mapping) => (
                    <MappingCard
                      key={mapping.name}
                      mapping={mapping}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                    />
                  ))
                )}
              </div>
            </div>

            {/* 右侧: Request */}
            <div>
              <h3 className="mb-2 text-xs font-medium uppercase text-orange-500">
                Request ({requestMappings.length})
              </h3>
              <div className="space-y-2">
                {requestMappings.length === 0 ? (
                  <p className="text-xs text-muted-foreground">No request mappings</p>
                ) : (
                  requestMappings.map((mapping) => (
                    <MappingCard
                      key={mapping.name}
                      mapping={mapping}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                    />
                  ))
                )}
              </div>
            </div>
          </div>
        )}
      </PageContainer>

      <MappingForm
        open={showForm}
        onClose={handleCloseForm}
        mapping={useMappingStore.getState().editingMapping}
      />
    </div>
  )
}

interface MappingCardProps {
  mapping: MappingRule
  onEdit: (mapping: MappingRule) => void
  onDelete: (name: string) => void
}

function MappingCard({ mapping, onEdit, onDelete }: MappingCardProps) {
  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        mapping.builtin
          ? 'border-blue-500/30 bg-blue-500/5'
          : mapping.enabled
            ? 'border-green-500/30 bg-green-500/5'
            : 'border-muted bg-muted/30'
      )}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 flex-wrap">
          {mapping.builtin && <Lock className="h-3.5 w-3.5 text-blue-500 flex-shrink-0" />}
          <span className="font-medium text-sm truncate">{mapping.name}</span>
          {mapping.builtin && (
            <span className="rounded bg-blue-500/20 px-1.5 py-0.5 text-xs text-blue-500">
              builtin
            </span>
          )}
          <span
            className={cn(
              'rounded px-1.5 py-0.5 text-xs',
              mapping.enabled
                ? 'bg-green-500/20 text-green-500'
                : 'bg-muted text-muted-foreground'
            )}
          >
            {mapping.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          {!mapping.builtin && (
            <>
              <button
                onClick={() => onEdit(mapping)}
                className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                title="Edit"
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => onDelete(mapping.name)}
                className="rounded p-1.5 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                title="Delete"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </>
          )}
        </div>
      </div>

      <div className="mt-2 space-y-1 text-sm">
        {mapping.description && (
          <p className="text-xs text-muted-foreground">{mapping.description}</p>
        )}
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground text-xs">Transformer:</span>
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium truncate">
            {mapping.transformer}
          </code>
        </div>
        {mapping.tags && mapping.tags.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">Tags:</span>
            <div className="flex flex-wrap gap-1">
              {mapping.tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded bg-gray-500/20 px-1.5 py-0.5 text-xs text-gray-600 dark:text-gray-400"
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>
        )}
        {mapping.exclude_tags && mapping.exclude_tags.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">Exclude:</span>
            <div className="flex flex-wrap gap-1">
              {mapping.exclude_tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded bg-red-500/20 px-1.5 py-0.5 text-xs text-red-600 dark:text-red-400"
                >
                  !{tag}
                </span>
              ))}
            </div>
          </div>
        )}
        {mapping.gateways && mapping.gateways.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">Gateways:</span>
            <div className="flex flex-wrap gap-1">
              {mapping.gateways.map((gw) => (
                <span
                  key={gw}
                  className="rounded bg-cyan-500/20 px-1.5 py-0.5 text-xs text-cyan-600 dark:text-cyan-400"
                >
                  {gw}
                </span>
              ))}
            </div>
          </div>
        )}
        {mapping.tools && mapping.tools.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">Tools ({mapping.tool_op || 'all'}):</span>
            <div className="flex flex-wrap gap-1">
              {mapping.tools.map((tool) => (
                <span
                  key={tool}
                  className="rounded bg-amber-500/20 px-1.5 py-0.5 text-xs text-amber-600 dark:text-amber-400 font-mono"
                >
                  {tool}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
