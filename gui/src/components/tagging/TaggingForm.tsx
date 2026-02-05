import { useState, useEffect, useMemo } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import CreatableSelect from 'react-select/creatable'
import { components } from 'react-select'
import type { ClassNamesConfig, GroupBase, OptionProps } from 'react-select'
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogFooter,
  ResizableDialogTitle,
} from '../ui/resizable-dialog'
import { Input, Checkbox, Button, Select } from '../ui/dialog'
import { useTaggingStore, TaggingRule, Matcher, MatchOp, MatcherType, TagMatchOp } from '../../store/tagging-store'
import { useConnectionStore } from '../../store/connection-store'

interface TagOption {
  value: string
  label: string
}

const selectClassNames: ClassNamesConfig<TagOption, boolean, GroupBase<TagOption>> = {
  control: (state) =>
    `!min-h-9 !rounded-md !border !border-input !bg-background !shadow-sm ${state.isFocused ? '!ring-1 !ring-ring !border-input' : ''}`,
  valueContainer: () => '!px-3 !py-0',
  input: () => '!m-0 !p-0 !text-sm',
  placeholder: () => '!text-muted-foreground !text-sm',
  singleValue: () => '!text-sm',
  multiValue: () => '!bg-accent !rounded !mr-1 !mb-1',
  multiValueLabel: () => '!text-sm !px-1.5 !py-0.5',
  multiValueRemove: () => '!rounded-r hover:!bg-destructive/20 hover:!text-destructive',
  menu: () => '!bg-popover !border !border-input !rounded-md !shadow-md !z-50',
  menuList: () => '!p-1',
  option: (state) =>
    `!rounded !px-2 !py-1.5 !text-sm !cursor-pointer ${state.isFocused ? '!bg-accent !text-accent-foreground' : ''} ${state.isSelected ? '!bg-primary !text-primary-foreground' : ''}`,
  indicatorSeparator: () => '!hidden',
  dropdownIndicator: () => '!p-1 !text-muted-foreground',
  clearIndicator: () => '!p-1 !text-muted-foreground hover:!text-foreground',
}

function CustomOption<IsMulti extends boolean>(props: OptionProps<TagOption, IsMulti, GroupBase<TagOption>>) {
  return (
    <div
      onPointerDown={(e) => {
        e.preventDefault()
        e.stopPropagation()
        props.selectOption(props.data)
      }}
    >
      <components.Option {...props} />
    </div>
  )
}

interface TaggingFormProps {
  open: boolean
  onClose: () => void
  rule: TaggingRule | null
}

const MATCH_OPS: { value: MatchOp; label: string }[] = [
  { value: 'exists', label: 'Exists' },
  { value: 'not_exists', label: 'Not Exists' },
  { value: 'eq', label: 'Equals' },
  { value: 'ne', label: 'Not Equals' },
  { value: 'contains', label: 'Contains' },
  { value: 'not_contains', label: 'Not Contains' },
  { value: 'prefix', label: 'Prefix' },
  { value: 'suffix', label: 'Suffix' },
  { value: 'regex', label: 'Regex' },
  { value: 'in', label: 'In List' },
  { value: 'not_in', label: 'Not In List' },
]

function createEmptyMatcher(): Matcher {
  return {
    type: 'header',
    key: '',
    match: { op: 'contains', value: '' },
  }
}

export function TaggingForm({ open, onClose, rule }: TaggingFormProps) {
  const { createRule, updateRule, error, rules, actualTags, fetchActualTags } = useTaggingStore()
  const { apiUrl } = useConnectionStore()

  const availableTags = useMemo(() => {
    const tagSet = new Set<string>()
    rules.forEach((r) => r.tags.forEach((t) => tagSet.add(t)))
    actualTags.forEach((t) => tagSet.add(t.name))
    return Array.from(tagSet).sort()
  }, [rules, actualTags])

  useEffect(() => {
    fetchActualTags(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const [name, setName] = useState('')
  const [priority, setPriority] = useState(100)
  const [enabled, setEnabled] = useState(true)
  const [matchers, setMatchers] = useState<Matcher[]>([createEmptyMatcher()])
  const [tags, setTags] = useState<string[]>([''])
  const [saving, setSaving] = useState(false)

  const isEditing = !!rule

  useEffect(() => {
    if (rule) {
      setName(rule.name)
      setPriority(rule.priority)
      setEnabled(rule.enabled)
      setMatchers(rule.matchers.length > 0 ? rule.matchers : [createEmptyMatcher()])
      setTags(rule.tags.length > 0 ? rule.tags : [''])
    } else {
      setName('')
      setPriority(100)
      setEnabled(true)
      setMatchers([createEmptyMatcher()])
      setTags([''])
    }
  }, [rule, open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)

    const validMatchers = matchers.filter((m) => {
      if (m.type === 'header') return m.key && m.match?.op
      if (m.type === 'tag') return m.tag
      if (m.type === 'tags') return m.tags && m.tags.length > 0
      return false
    })

    const validTags = tags.filter((t) => t.trim())

    const newRule: TaggingRule = {
      name,
      priority,
      enabled,
      builtin: false,
      matchers: validMatchers,
      tags: validTags,
    }

    let success: boolean
    if (isEditing) {
      success = await updateRule(apiUrl, rule.name, newRule)
    } else {
      success = await createRule(apiUrl, newRule)
    }

    setSaving(false)
    if (success) {
      onClose()
    }
  }

  const updateMatcher = (index: number, updates: Partial<Matcher>) => {
    setMatchers((prev) => {
      const newMatchers = [...prev]
      newMatchers[index] = { ...newMatchers[index], ...updates }
      return newMatchers
    })
  }

  const addMatcher = () => {
    setMatchers((prev) => [...prev, createEmptyMatcher()])
  }

  const removeMatcher = (index: number) => {
    setMatchers((prev) => prev.filter((_, i) => i !== index))
  }

  const updateTag = (index: number, value: string) => {
    setTags((prev) => {
      const newTags = [...prev]
      newTags[index] = value
      return newTags
    })
  }

  const addTag = () => {
    setTags((prev) => [...prev, ''])
  }

  const removeTag = (index: number) => {
    setTags((prev) => prev.filter((_, i) => i !== index))
  }

  return (
    <ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <ResizableDialogContent
        defaultWidth={550}
        defaultHeight={600}
        minWidth={450}
        minHeight={500}
      >
        <ResizableDialogHeader>
          <ResizableDialogTitle>{isEditing ? 'Edit Tag Rule' : 'Add Tag Rule'}</ResizableDialogTitle>
        </ResizableDialogHeader>
        <ResizableDialogBody>
          <form id="tagging-form" onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <Input
                label="Name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="tag_name"
                required
                disabled={isEditing}
              />
              <Input
                label="Priority"
                type="number"
                value={priority}
                onChange={(e) => setPriority(parseInt(e.target.value) || 0)}
                min={0}
                required
              />
            </div>

            <Checkbox
              label="Enabled"
              checked={enabled}
              onChange={setEnabled}
            />

            <div>
              <div className="mb-2 flex items-center justify-between">
                <label className="text-sm font-medium">Matchers</label>
                <button
                  type="button"
                  onClick={addMatcher}
                  className="flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent"
                >
                  <Plus className="h-3 w-3" /> Add Matcher
                </button>
              </div>
              <div className="space-y-2">
                {matchers.map((matcher, index) => (
                  <MatcherEditor
                    key={index}
                    matcher={matcher}
                    onChange={(updates) => updateMatcher(index, updates)}
                    onRemove={() => removeMatcher(index)}
                    canRemove={matchers.length > 1}
                    availableTags={availableTags}
                  />
                ))}
              </div>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <label className="text-sm font-medium">Output Tags</label>
                <button
                  type="button"
                  onClick={addTag}
                  className="flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent"
                >
                  <Plus className="h-3 w-3" /> Add Tag
                </button>
              </div>
              <div className="space-y-2">
                {tags.map((tag, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={tag}
                      onChange={(e) => updateTag(index, e.target.value)}
                      className="flex-1 rounded-md border bg-background px-3 py-2 text-sm"
                      placeholder="output_tag_name"
                    />
                    {tags.length > 1 && (
                      <button
                        type="button"
                        onClick={() => removeTag(index)}
                        className="rounded p-1.5 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            </div>

            {error && <p className="text-sm text-red-500">{error}</p>}
          </form>
        </ResizableDialogBody>
        <ResizableDialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="tagging-form" disabled={saving}>
            {saving ? 'Saving...' : isEditing ? 'Update' : 'Create'}
          </Button>
        </ResizableDialogFooter>
      </ResizableDialogContent>
    </ResizableDialog>
  )
}

interface MatcherEditorProps {
  matcher: Matcher
  onChange: (updates: Partial<Matcher>) => void
  onRemove: () => void
  canRemove: boolean
  availableTags: string[]
}

function MatcherEditor({ matcher, onChange, onRemove, canRemove, availableTags }: MatcherEditorProps) {
  const handleTypeChange = (type: MatcherType) => {
    if (type === 'header') {
      onChange({ type, key: '', match: { op: 'contains', value: '' }, tag: undefined, tags: undefined, tag_op: undefined })
    } else if (type === 'tag') {
      onChange({ type, tag: '', key: undefined, match: undefined, tags: undefined, tag_op: undefined })
    } else if (type === 'tags') {
      onChange({ type, tags: [''], tag_op: 'all', key: undefined, match: undefined, tag: undefined })
    }
  }

  return (
    <div className="rounded-md border bg-muted/30 p-3">
      <div className="space-y-2">
        <div className="flex items-start gap-2">
          <Select
            value={matcher.type}
            onChange={(e) => handleTypeChange(e.target.value as MatcherType)}
            className="w-auto h-9"
          >
            <option value="header">Header</option>
            <option value="tag">Tag</option>
            <option value="tags">Tags</option>
          </Select>

          <div className="flex-1">
            {matcher.type === 'header' && (
              <input
                type="text"
                value={matcher.key || ''}
                onChange={(e) => onChange({ key: e.target.value })}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                placeholder="Header Key"
              />
            )}
            {matcher.type === 'tag' && (
              <TagMatcherFields matcher={matcher} onChange={onChange} availableTags={availableTags} />
            )}
            {matcher.type === 'tags' && (
              <TagsMatcherFields matcher={matcher} onChange={onChange} availableTags={availableTags} />
            )}
          </div>

          {canRemove && (
            <button
              type="button"
              onClick={onRemove}
              className="rounded p-1.5 h-9 flex items-center text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
        </div>

        {matcher.type === 'header' && (
          <HeaderMatcherOpFields matcher={matcher} onChange={onChange} />
        )}
      </div>
    </div>
  )
}

function HeaderMatcherOpFields({ matcher, onChange }: { matcher: Matcher; onChange: (updates: Partial<Matcher>) => void }) {
  const needsValue = matcher.match?.op && !['exists', 'not_exists'].includes(matcher.match.op)
  const needsValues = matcher.match?.op && ['in', 'not_in'].includes(matcher.match.op)

  return (
    <div className="flex gap-2">
      <Select
        value={matcher.match?.op || 'contains'}
        onChange={(e) => onChange({ match: { ...matcher.match, op: e.target.value as MatchOp } })}
        className="w-auto"
      >
        {MATCH_OPS.map((op) => (
          <option key={op.value} value={op.value}>
            {op.label}
          </option>
        ))}
      </Select>
      {needsValue && !needsValues && (
        <input
          type="text"
          value={matcher.match?.value || ''}
          onChange={(e) => onChange({ match: { ...matcher.match!, value: e.target.value } })}
          className="flex-1 h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
          placeholder="Value"
        />
      )}
      {needsValues && (
        <input
          type="text"
          value={matcher.match?.values?.join(', ') || ''}
          onChange={(e) => onChange({ match: { ...matcher.match!, values: e.target.value.split(',').map((s) => s.trim()) } })}
          className="flex-1 h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
          placeholder="value1, value2, ..."
        />
      )}
    </div>
  )
}

function TagMatcherFields({ matcher, onChange, availableTags }: { matcher: Matcher; onChange: (updates: Partial<Matcher>) => void; availableTags: string[] }) {
  const options = availableTags.map((t) => ({ value: t, label: t }))
  const selectedOption = matcher.tag ? { value: matcher.tag, label: matcher.tag } : null

  return (
    <CreatableSelect<TagOption, false>
      isClearable
      options={options}
      value={selectedOption}
      onChange={(opt) => onChange({ tag: opt?.value || '' })}
      onCreateOption={(inputValue) => onChange({ tag: inputValue })}
      placeholder="Select or type tag..."
      classNames={selectClassNames}
      components={{ Option: CustomOption }}
      unstyled
    />
  )
}

function TagsMatcherFields({ matcher, onChange, availableTags }: { matcher: Matcher; onChange: (updates: Partial<Matcher>) => void; availableTags: string[] }) {
  const options = availableTags.map((t) => ({ value: t, label: t }))
  const selectedOptions = (matcher.tags || []).filter((t) => t).map((t) => ({ value: t, label: t }))

  return (
    <div className="flex flex-wrap gap-2">
      <Select
        value={matcher.tag_op || 'all'}
        onChange={(e) => onChange({ tag_op: e.target.value as TagMatchOp })}
        className="w-auto"
      >
        <option value="all">All (AND)</option>
        <option value="any">Any (OR)</option>
      </Select>
      <div className="flex-1 min-w-[200px]">
        <CreatableSelect<TagOption, true>
          isMulti
          isClearable
          options={options}
          value={selectedOptions}
          onChange={(opts) => onChange({ tags: opts.map((o) => o.value) })}
          onCreateOption={(inputValue) => onChange({ tags: [...(matcher.tags || []), inputValue] })}
          placeholder="Select or type tags..."
          classNames={selectClassNames}
          components={{ Option: CustomOption }}
          unstyled
        />
      </div>
    </div>
  )
}
