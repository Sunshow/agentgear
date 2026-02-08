import { useEffect, useMemo } from 'react'
import { Plus, Pencil, Trash2, Lock, ArrowRight, Copy } from 'lucide-react'
import { useTransformerStore, TransformerDef } from '../../store/transformer-store'
import { useConnectionStore } from '../../store/connection-store'
import { TransformerForm } from './TransformerForm'
import { PageContainer } from '../layout/PageContainer'
import { cn } from '../../lib/utils'

export function TransformerList() {
  const { definitions, loading, fetchDefinitions, deleteDefinition, showForm, setShowForm, setEditingDef, setSelectedTemplate } =
    useTransformerStore()
  const { apiUrl } = useConnectionStore()

  useEffect(() => {
    fetchDefinitions(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const { templates, responseTransformers, requestTransformers } = useMemo(() => {
    return {
      templates: definitions.filter(d => d.is_template),
      responseTransformers: definitions.filter(d => !d.is_template && d.direction === 'response'),
      requestTransformers: definitions.filter(d => !d.is_template && d.direction === 'request'),
    }
  }, [definitions])

  const handleAdd = () => {
    setEditingDef(null)
    setSelectedTemplate(null)
    setShowForm(true)
  }

  const handleEdit = (def: TransformerDef) => {
    setEditingDef(def)
    setSelectedTemplate(null)
    setShowForm(true)
  }

  const handleCreateFromTemplate = (template: TransformerDef) => {
    setEditingDef(null)
    setSelectedTemplate(template)
    setShowForm(true)
  }

  const handleDelete = async (name: string) => {
    if (confirm(`Are you sure you want to delete transformer "${name}"?`)) {
      await deleteDefinition(apiUrl, name)
    }
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingDef(null)
    setSelectedTemplate(null)
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
        <h2 className="text-sm font-semibold">Transformers</h2>
        <button
          onClick={handleAdd}
          className="flex h-7 items-center gap-1 rounded-md border bg-background px-2 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Plus className="h-3.5 w-3.5" />
          Add
        </button>
      </div>

      <PageContainer className="p-4">
        {definitions.length === 0 ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            No transformers configured
          </div>
        ) : (
          <div className="space-y-4">
            {/* Templates Section */}
            {templates.length > 0 && (
              <div>
                <h3 className="mb-2 text-xs font-medium uppercase text-yellow-600 dark:text-yellow-400">
                  Templates ({templates.length})
                </h3>
                <div className="space-y-2">
                  {templates.map((def) => (
                    <TransformerCard
                      key={def.name}
                      def={def}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                      onCreateFromTemplate={handleCreateFromTemplate}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Response / Request 左右分栏 */}
            <div className="grid grid-cols-2 gap-4">
              {/* 左侧: Response */}
              <div>
                <h3 className="mb-2 text-xs font-medium uppercase text-purple-500">
                  Response ({responseTransformers.length})
                </h3>
                <div className="space-y-2">
                  {responseTransformers.length === 0 ? (
                    <p className="text-xs text-muted-foreground">No response transformers</p>
                  ) : (
                    responseTransformers.map((def) => (
                      <TransformerCard
                        key={def.name}
                        def={def}
                        onEdit={handleEdit}
                        onDelete={handleDelete}
                        onCreateFromTemplate={handleCreateFromTemplate}
                      />
                    ))
                  )}
                </div>
              </div>

              {/* 右侧: Request */}
              <div>
                <h3 className="mb-2 text-xs font-medium uppercase text-orange-500">
                  Request ({requestTransformers.length})
                </h3>
                <div className="space-y-2">
                  {requestTransformers.length === 0 ? (
                    <p className="text-xs text-muted-foreground">No request transformers</p>
                  ) : (
                    requestTransformers.map((def) => (
                      <TransformerCard
                        key={def.name}
                        def={def}
                        onEdit={handleEdit}
                        onDelete={handleDelete}
                        onCreateFromTemplate={handleCreateFromTemplate}
                      />
                    ))
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </PageContainer>

      <TransformerForm
        open={showForm}
        onClose={handleCloseForm}
        definition={useTransformerStore.getState().editingDef}
      />
    </div>
  )
}

interface TransformerCardProps {
  def: TransformerDef
  onEdit: (def: TransformerDef) => void
  onDelete: (name: string) => void
  onCreateFromTemplate: (template: TransformerDef) => void
}

function TransformerCard({ def, onEdit, onDelete, onCreateFromTemplate }: TransformerCardProps) {
  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        def.builtin
          ? 'border-blue-500/30 bg-blue-500/5'
          : 'border-green-500/30 bg-green-500/5'
      )}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 flex-wrap">
          {def.builtin && <Lock className="h-3.5 w-3.5 text-blue-500 flex-shrink-0" />}
          <span className="font-medium text-sm truncate">{def.name}</span>
          {def.builtin && (
            <span className="rounded bg-blue-500/20 px-1.5 py-0.5 text-xs text-blue-500">
              builtin
            </span>
          )}
          {def.is_template && (
            <span className="rounded bg-yellow-500/20 px-1.5 py-0.5 text-xs text-yellow-600 dark:text-yellow-400">
              template
            </span>
          )}
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          {def.is_template && (
            <button
              onClick={() => onCreateFromTemplate(def)}
              className="flex items-center gap-1 rounded p-1.5 text-muted-foreground hover:bg-green-500/10 hover:text-green-500"
              title="Create from this template"
            >
              <Copy className="h-3.5 w-3.5" />
            </button>
          )}
          {!def.builtin && (
            <>
              <button
                onClick={() => onEdit(def)}
                className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                title="Edit"
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => onDelete(def.name)}
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
        {def.description && (
          <p className="text-xs text-muted-foreground">{def.description}</p>
        )}
        {def.is_template ? (
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">Parameters:</span>
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              {`{{direction}}, {{source}}, {{target}}`}
            </code>
          </div>
        ) : (
          <>
            {/* Type badge */}
            {def.type && def.type !== 'tool' && (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-xs">Type:</span>
                <span className={cn(
                  "rounded px-1.5 py-0.5 text-xs",
                  def.type === 'compress' && "bg-cyan-500/20 text-cyan-600 dark:text-cyan-400",
                  def.type === 'message_inject' && "bg-green-500/20 text-green-600 dark:text-green-400",
                  def.type === 'error_transform' && "bg-red-500/20 text-red-600 dark:text-red-400",
                  def.type === 'header_inject' && "bg-purple-500/20 text-purple-600 dark:text-purple-400"
                )}>
                  {def.type}
                </span>
              </div>
            )}
            
            {/* Tool transform fields */}
            {(def.source_tool || def.target_tool) && (
              <div className="flex items-center gap-2">
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{def.source_tool || '-'}</code>
                <ArrowRight className="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{def.target_tool || '-'}</code>
              </div>
            )}
            {def.accumulate && (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-xs">Accumulate:</span>
                <span className="rounded bg-yellow-500/20 px-1.5 py-0.5 text-xs text-yellow-600 dark:text-yellow-400">
                  enabled
                </span>
              </div>
            )}
            
            {/* Compress fields */}
            {def.type === 'compress' && (
              <>
                {def.compress_model && (
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">Model:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{def.compress_model}</code>
                  </div>
                )}
                {def.compress_target && (
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">Target:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{def.compress_target}</code>
                  </div>
                )}
                {def.context_token_limit && (
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">Token Limit:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                      {def.context_token_limit.toLocaleString()} @ {((def.context_threshold_ratio || 0.7) * 100).toFixed(0)}%
                    </code>
                  </div>
                )}
                {def.preserve_budget && (
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">Preserve:</span>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{def.preserve_budget.toLocaleString()} tokens</code>
                  </div>
                )}
                {def.auto_retry && (
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground text-xs">Auto Retry:</span>
                    <span className="rounded bg-green-500/20 px-1.5 py-0.5 text-xs text-green-600 dark:text-green-400">
                      enabled
                    </span>
                  </div>
                )}
              </>
            )}
            
            {def.param_mapping && def.param_mapping.length > 0 && (
              <div className="flex items-start gap-2">
                <span className="text-muted-foreground text-xs">Mappings:</span>
                <div className="flex flex-wrap gap-1">
                  {def.param_mapping.map((pm, i) => (
                    <code key={i} className="rounded bg-muted px-1.5 py-0.5 text-xs">
                      {pm.from} → {pm.to}
                      {pm.transform && ` (${pm.transform})`}
                    </code>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
