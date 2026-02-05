import { groupTagsByCategory, TAG_CATEGORY_ORDER, TAG_CATEGORY_LABELS } from '../../lib/tag-colors'
import { TagBadge } from './tag-badge'

interface TagGroupProps {
  tags: string[]
  showLabels?: boolean
  size?: 'xs' | 'sm'
  counts?: Record<string, number>
}

export function TagGroup({ tags, showLabels = true, size = 'sm', counts }: TagGroupProps) {
  const grouped = groupTagsByCategory(tags)

  return (
    <div className="space-y-2">
      {TAG_CATEGORY_ORDER.map((category) => {
        const categoryTags = grouped[category]
        if (categoryTags.length === 0) return null

        return (
          <div key={category} className="flex flex-wrap items-center gap-1.5">
            {showLabels && (
              <span className="text-xs font-medium text-muted-foreground w-16 shrink-0">
                {TAG_CATEGORY_LABELS[category]}
              </span>
            )}
            <div className="flex flex-wrap gap-1">
              {categoryTags.map((tag) => (
                <TagBadge
                  key={tag}
                  tag={tag}
                  size={size}
                  count={counts?.[tag]}
                />
              ))}
            </div>
          </div>
        )
      })}
    </div>
  )
}

interface TagGroupCompactProps {
  tags: string[]
  maxTags?: number
  size?: 'xs' | 'sm'
}

export function TagGroupCompact({ tags, maxTags = 3, size = 'xs' }: TagGroupCompactProps) {
  const grouped = groupTagsByCategory(tags)
  const sortedTags: string[] = []

  for (const category of TAG_CATEGORY_ORDER) {
    sortedTags.push(...grouped[category])
  }

  const displayTags = sortedTags.slice(0, maxTags)
  const remaining = sortedTags.length - maxTags

  return (
    <div className="flex flex-wrap items-center gap-1">
      {displayTags.map((tag) => (
        <TagBadge key={tag} tag={tag} size={size} />
      ))}
      {remaining > 0 && (
        <span className="text-[10px] text-muted-foreground">+{remaining}</span>
      )}
    </div>
  )
}
