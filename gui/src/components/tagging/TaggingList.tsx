import { useEffect } from 'react'
import { Plus, Pencil, Trash2, Lock } from 'lucide-react'
import { useTaggingStore, TaggingRule, Matcher } from '../../store/tagging-store'
import { useConnectionStore } from '../../store/connection-store'
import { TaggingForm } from './TaggingForm'
import { PageContainer } from '../layout/PageContainer'
import { cn } from '../../lib/utils'
import { TagBadge } from '../ui/tag-badge'
import { TagGroup } from '../ui/tag-group'

function formatMatcher(m: Matcher): string {
  switch (m.type) {
    case 'header':
      return `header(${m.key}) ${m.match?.op || ''} ${m.match?.value ? `"${m.match.value}"` : ''}`
    case 'tag':
      return `tag: ${m.tag}`
    case 'tags':
      return `tags(${m.tag_op || 'all'}): ${m.tags?.join(', ')}`
    default:
      return 'unknown'
  }
}

export function TaggingList() {
  const { rules, loading, fetchRules, actualTags, fetchActualTags, deleteRule, showForm, setShowForm, setEditingRule } = useTaggingStore()
  const { apiUrl } = useConnectionStore()

  useEffect(() => {
    fetchRules(apiUrl)
    fetchActualTags(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const handleAdd = () => {
    setEditingRule(null)
    setShowForm(true)
  }

  const handleEdit = (rule: TaggingRule) => {
    setEditingRule(rule)
    setShowForm(true)
  }

  const handleDelete = async (name: string) => {
    if (confirm(`Are you sure you want to delete rule "${name}"?`)) {
      await deleteRule(apiUrl, name)
    }
  }

  const handleCloseForm = () => {
    setShowForm(false)
    setEditingRule(null)
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
        <h2 className="text-sm font-semibold">Taggings</h2>
        <button
          onClick={handleAdd}
          className="flex h-7 items-center gap-1 rounded-md border bg-background px-2 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <Plus className="h-3.5 w-3.5" />
          Add Rule
        </button>
      </div>

      <PageContainer className="p-4">
        {/* All Tags Section */}
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-muted-foreground">All Tags</h3>
          {actualTags.length === 0 ? (
            <p className="text-xs text-muted-foreground">No tags recorded yet. Send some requests first.</p>
          ) : (
            <TagGroup
              tags={actualTags.map((t) => t.name)}
              counts={Object.fromEntries(actualTags.map((t) => [t.name, t.count]))}
              showLabels={true}
              size="sm"
            />
          )}
        </div>

        {/* Tagging Rules Section */}
        <div>
          <h3 className="mb-2 text-sm font-medium text-muted-foreground">Tagging Rules</h3>
          {rules.length === 0 ? (
            <p className="text-xs text-muted-foreground">No tagging rules configured</p>
          ) : (
            <div className="space-y-3">
            {rules.map((rule) => (
              <div
                key={rule.name}
                className={cn(
                  'rounded-lg border p-4',
                  rule.builtin
                    ? 'border-blue-500/30 bg-blue-500/5'
                    : rule.enabled
                      ? 'border-green-500/30 bg-green-500/5'
                      : 'border-muted bg-muted/30'
                )}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    {rule.builtin && <Lock className="h-3.5 w-3.5 text-blue-500" />}
                    <span className="font-medium">{rule.name}</span>
                    {rule.builtin && (
                      <span className="rounded bg-blue-500/20 px-1.5 py-0.5 text-xs text-blue-500">
                        builtin
                      </span>
                    )}
                    <span
                      className={cn(
                        'rounded px-1.5 py-0.5 text-xs',
                        rule.enabled
                          ? 'bg-green-500/20 text-green-500'
                          : 'bg-muted text-muted-foreground'
                      )}
                    >
                      {rule.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </div>
                  <div className="flex items-center gap-1">
                    <span className="mr-2 text-xs text-muted-foreground">
                      Priority: {rule.priority}
                    </span>
                    {!rule.builtin && (
                      <>
                        <button
                          onClick={() => handleEdit(rule)}
                          className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                          title="Edit"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          onClick={() => handleDelete(rule.name)}
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
                  <div className="flex items-start gap-2">
                    <span className="text-muted-foreground">Matchers:</span>
                    <div className="flex flex-wrap gap-1">
                      {rule.matchers.map((m, i) => (
                        <code key={i} className="rounded bg-muted px-1.5 py-0.5 text-xs">
                          {formatMatcher(m)}
                        </code>
                      ))}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground">Tags:</span>
                    <div className="flex flex-wrap gap-1">
                      {rule.tags.map((tag) => (
                        <TagBadge key={tag} tag={tag} size="xs" />
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
          )}
        </div>
      </PageContainer>

      <TaggingForm open={showForm} onClose={handleCloseForm} rule={useTaggingStore.getState().editingRule} />
    </div>
  )
}
