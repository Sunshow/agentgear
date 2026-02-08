import { useState, useEffect, useMemo } from 'react'
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
import { useMappingStore, MappingRule } from '../../store/mapping-store'
import { useTransformerStore } from '../../store/transformer-store'
import { useTaggingStore } from '../../store/tagging-store'
import { useGatewayStore } from '../../store/gateway-store'
import { useConnectionStore } from '../../store/connection-store'

interface SelectOption {
  value: string
  label: string
}

const selectClassNames: ClassNamesConfig<SelectOption, boolean, GroupBase<SelectOption>> = {
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

function CustomOption<IsMulti extends boolean>(
  props: OptionProps<SelectOption, IsMulti, GroupBase<SelectOption>>
) {
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

interface MappingFormProps {
  open: boolean
  onClose: () => void
  mapping: MappingRule | null
}

export function MappingForm({ open, onClose, mapping }: MappingFormProps) {
  const { createMapping, updateMapping, error, prefilledTransformer } = useMappingStore()
  const { definitions, fetchDefinitions } = useTransformerStore()
  const { rules, fetchRules, actualTags, fetchActualTags } = useTaggingStore()
  const { gateways, fetchGateways } = useGatewayStore()
  const { apiUrl } = useConnectionStore()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [tags, setTags] = useState<string[]>([])
  const [excludeTags, setExcludeTags] = useState<string[]>([])
  const [selectedGateways, setSelectedGateways] = useState<string[]>([])
  const [tools, setTools] = useState<string[]>([])
  const [toolOp, setToolOp] = useState<'all' | 'any'>('all')
  const [transformer, setTransformer] = useState('')
  const [saving, setSaving] = useState(false)

  const isEditing = !!mapping

  const availableTags = useMemo(() => {
    const tagSet = new Set<string>()
    rules.forEach((r) => r.tags.forEach((t) => tagSet.add(t)))
    actualTags.forEach((t) => tagSet.add(t.name))
    return Array.from(tagSet).sort()
  }, [rules, actualTags])

  const gatewayOptions = useMemo(() => {
    return gateways.map((g) => ({ value: g.name, label: g.name }))
  }, [gateways])

  const transformerOptions = useMemo(() => {
    return definitions.map((d) => {
      let label = d.name
      if (d.type === 'compress') {
        label += ` (${d.direction}: compress)`
      } else if (d.type === 'message_inject') {
        label += ` (${d.direction}: message inject)`
      } else if (d.type === 'error_transform') {
        label += ` (${d.direction}: error transform)`
      } else if (d.type === 'header_inject') {
        label += ` (${d.direction}: header inject)`
      } else if (d.source_tool || d.target_tool) {
        label += ` (${d.direction}: ${d.source_tool || '*'} → ${d.target_tool || '*'})`
      } else {
        label += ` (${d.direction})`
      }
      return {
        value: d.name,
        label,
      }
    })
  }, [definitions])

  useEffect(() => {
    fetchDefinitions(apiUrl)
    fetchRules(apiUrl)
    fetchActualTags(apiUrl)
    fetchGateways(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  useEffect(() => {
    if (mapping) {
      setName(mapping.name)
      setDescription(mapping.description || '')
      setEnabled(mapping.enabled)
      setTags(mapping.tags || [])
      setExcludeTags(mapping.exclude_tags || [])
      setSelectedGateways(mapping.gateways || [])
      setTools(mapping.tools || [])
      setToolOp((mapping.tool_op as 'all' | 'any') || 'all')
      setTransformer(mapping.transformer)
    } else {
      setName('')
      setDescription('')
      setEnabled(true)
      setTags([])
      setExcludeTags([])
      setSelectedGateways([])
      setTools([])
      setToolOp('all')
      setTransformer(prefilledTransformer || '')
    }
  }, [mapping, open, prefilledTransformer])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)

    const newMapping: MappingRule = {
      name,
      description: description || undefined,
      enabled,
      tags,
      exclude_tags: excludeTags.length > 0 ? excludeTags : undefined,
      gateways: selectedGateways,
      tools: tools.length > 0 ? tools : undefined,
      tool_op: tools.length > 0 ? toolOp : undefined,
      transformer,
      builtin: false,
    }

    let success: boolean
    if (isEditing) {
      success = await updateMapping(apiUrl, mapping.name, newMapping)
    } else {
      success = await createMapping(apiUrl, newMapping)
    }

    setSaving(false)
    if (success) {
      onClose()
    }
  }

  return (
    <ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <ResizableDialogContent defaultWidth={500} defaultHeight={480} minWidth={400} minHeight={400}>
        <ResizableDialogHeader>
          <ResizableDialogTitle>{isEditing ? 'Edit Mapping' : 'Add Mapping'}</ResizableDialogTitle>
        </ResizableDialogHeader>
        <ResizableDialogBody>
          <form id="mapping-form" onSubmit={handleSubmit} className="space-y-4">
            <Input
              label="Name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="mapping_name"
              required
              disabled={isEditing}
            />

            <Input
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
            />

            <Checkbox label="Enabled" checked={enabled} onChange={setEnabled} />

            <div>
              <label className="mb-1.5 block text-sm font-medium">Transformer</label>
              <Select
                value={transformer}
                onChange={(e) => setTransformer(e.target.value)}
                className="w-full"
                required
              >
                <option value="">Select a transformer...</option>
                {transformerOptions.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </Select>
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium">Tags (conditions)</label>
              <CreatableSelect<SelectOption, true>
                isMulti
                isClearable
                options={availableTags.map((t) => ({ value: t, label: t }))}
                value={tags.map((t) => ({ value: t, label: t }))}
                onChange={(opts) => setTags(opts.map((o) => o.value))}
                onCreateOption={(inputValue) => setTags([...tags, inputValue])}
                placeholder="Select or type tags..."
                classNames={selectClassNames}
                components={{ Option: CustomOption }}
                unstyled
              />
              <p className="mt-1 text-xs text-muted-foreground">
                All specified tags must be present (AND logic)
              </p>
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium">Exclude Tags (conditions)</label>
              <CreatableSelect<SelectOption, true>
                isMulti
                isClearable
                options={availableTags.map((t) => ({ value: t, label: t }))}
                value={excludeTags.map((t) => ({ value: t, label: t }))}
                onChange={(opts) => setExcludeTags(opts.map((o) => o.value))}
                onCreateOption={(inputValue) => setExcludeTags([...excludeTags, inputValue])}
                placeholder="Select or type tags to exclude..."
                classNames={selectClassNames}
                components={{ Option: CustomOption }}
                unstyled
              />
              <p className="mt-1 text-xs text-muted-foreground">
                None of these tags must be present (NOT logic)
              </p>
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium">Gateways (conditions)</label>
              <CreatableSelect<SelectOption, true>
                isMulti
                isClearable
                options={gatewayOptions}
                value={selectedGateways.map((g) => ({ value: g, label: g }))}
                onChange={(opts) => setSelectedGateways(opts.map((o) => o.value))}
                onCreateOption={(inputValue) => setSelectedGateways([...selectedGateways, inputValue])}
                placeholder="Select gateways..."
                classNames={selectClassNames}
                components={{ Option: CustomOption }}
                unstyled
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Leave empty to match all gateways
              </p>
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium">Tools (conditions)</label>
              <CreatableSelect<SelectOption, true>
                isMulti
                isClearable
                options={[]}
                value={tools.map((t) => ({ value: t, label: t }))}
                onChange={(opts) => setTools(opts.map((o) => o.value))}
                onCreateOption={(inputValue) => setTools([...tools, inputValue])}
                placeholder="Select or type tool names..."
                classNames={selectClassNames}
                components={{ Option: CustomOption }}
                unstyled
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Leave empty to match all tools
              </p>
            </div>

            {tools.length > 0 && (
              <div>
                <label className="mb-1.5 block text-sm font-medium">Tool Match Mode</label>
                <Select
                  value={toolOp}
                  onChange={(e) => setToolOp(e.target.value as 'all' | 'any')}
                  className="w-full"
                >
                  <option value="all">All (must match all specified tools)</option>
                  <option value="any">Any (match any of the specified tools)</option>
                </Select>
              </div>
            )}

            {error && <p className="text-sm text-red-500">{error}</p>}
          </form>
        </ResizableDialogBody>
        <ResizableDialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="mapping-form" disabled={saving}>
            {saving ? 'Saving...' : isEditing ? 'Update' : 'Create'}
          </Button>
        </ResizableDialogFooter>
      </ResizableDialogContent>
    </ResizableDialog>
  )
}
