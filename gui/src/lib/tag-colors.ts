export type TagCategory = 'agent' | 'protocol' | 'upstream' | 'gateway' | 'tool' | 'custom'

export const TAG_CATEGORY_ORDER: TagCategory[] = ['agent', 'protocol', 'upstream', 'gateway', 'tool', 'custom']

export const TAG_CATEGORY_LABELS: Record<TagCategory, string> = {
  agent: 'Agent',
  protocol: 'Protocol',
  upstream: 'Upstream',
  gateway: 'Gateway',
  tool: 'Tool',
  custom: 'Custom',
}

export const TAG_COLORS: Record<TagCategory, string> = {
  agent: 'bg-purple-500/20 text-purple-700 dark:text-purple-400',
  protocol: 'bg-blue-500/20 text-blue-700 dark:text-blue-400',
  upstream: 'bg-amber-500/20 text-amber-700 dark:text-amber-400',
  gateway: 'bg-cyan-500/20 text-cyan-700 dark:text-cyan-400',
  tool: 'bg-green-500/20 text-green-700 dark:text-green-400',
  custom: 'bg-rose-500/20 text-rose-700 dark:text-rose-400',
}

export function getTagCategory(tag: string): TagCategory {
  if (tag.startsWith('$a_')) return 'agent'
  if (tag.startsWith('$p_')) return 'protocol'
  if (tag.startsWith('$u_')) return 'upstream'
  if (tag.startsWith('$g_')) return 'gateway'
  if (tag.startsWith('$t_')) return 'tool'
  return 'custom'
}

export function sortTagsByCategory(tags: string[]): string[] {
  return [...tags].sort((a, b) => {
    const catA = TAG_CATEGORY_ORDER.indexOf(getTagCategory(a))
    const catB = TAG_CATEGORY_ORDER.indexOf(getTagCategory(b))
    if (catA !== catB) return catA - catB
    return a.localeCompare(b)
  })
}

export function groupTagsByCategory(tags: string[]): Record<TagCategory, string[]> {
  const grouped: Record<TagCategory, string[]> = {
    agent: [],
    protocol: [],
    upstream: [],
    gateway: [],
    tool: [],
    custom: [],
  }
  for (const tag of tags) {
    grouped[getTagCategory(tag)].push(tag)
  }
  for (const cat of TAG_CATEGORY_ORDER) {
    grouped[cat].sort((a, b) => a.localeCompare(b))
  }
  return grouped
}
