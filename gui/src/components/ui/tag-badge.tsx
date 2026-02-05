import { cn } from '../../lib/utils'
import { getTagCategory, TAG_COLORS } from '../../lib/tag-colors'

interface TagBadgeProps {
  tag: string
  count?: number
  size?: 'xs' | 'sm'
}

export function TagBadge({ tag, count, size = 'sm' }: TagBadgeProps) {
  const category = getTagCategory(tag)
  const colorClass = TAG_COLORS[category]

  return (
    <span
      className={cn(
        'inline-flex items-center rounded font-medium',
        colorClass,
        size === 'xs' ? 'px-1.5 py-0.5 text-[10px] gap-1' : 'px-2 py-1 text-xs gap-1.5'
      )}
    >
      {tag}
      {count !== undefined && (
        <span
          className={cn(
            'rounded-full bg-black/10 dark:bg-white/10',
            size === 'xs' ? 'px-1 py-0 text-[9px]' : 'px-1.5 py-0.5 text-[10px]'
          )}
        >
          {count}
        </span>
      )}
    </span>
  )
}
