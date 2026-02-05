import { useState, useEffect, useMemo } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogFooter,
  ResizableDialogTitle,
} from '../ui/resizable-dialog'
import { Input, Checkbox, Button, Select } from '../ui/dialog'
import { useTransformerStore, TransformerDef, ParamMapping } from '../../store/transformer-store'
import { useConnectionStore } from '../../store/connection-store'

interface TransformerFormProps {
  open: boolean
  onClose: () => void
  definition: TransformerDef | null
}

function createEmptyParamMapping(): ParamMapping {
  return { from: '', to: '', transform: '' }
}

function extractTemplateParams(template: TransformerDef): string[] {
  const params = new Set<string>()
  const regex = /\{\{(\w+)\}\}/g
  
  const fields = [template.direction, template.source_tool, template.target_tool]
  fields.forEach(field => {
    if (field) {
      let match
      while ((match = regex.exec(field)) !== null) {
        params.add(match[1])
      }
    }
  })
  
  return Array.from(params)
}

export function TransformerForm({ open, onClose, definition }: TransformerFormProps) {
  const { createDefinition, updateDefinition, error, selectedTemplate } = useTransformerStore()
  const { apiUrl } = useConnectionStore()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [direction, setDirection] = useState<'request' | 'response'>('response')
  const [sourceTool, setSourceTool] = useState('')
  const [targetTool, setTargetTool] = useState('')
  const [accumulate, setAccumulate] = useState(false)
  const [paramMapping, setParamMapping] = useState<ParamMapping[]>([])
  const [isTemplate, setIsTemplate] = useState(false)
  const [templateArgs, setTemplateArgs] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  const isEditing = !!definition
  const isFromTemplate = !!selectedTemplate

  const templateParams = useMemo(() => {
    if (selectedTemplate) {
      return extractTemplateParams(selectedTemplate)
    }
    return []
  }, [selectedTemplate])

  useEffect(() => {
    if (definition) {
      setName(definition.name)
      setDescription(definition.description || '')
      setDirection(definition.direction)
      setSourceTool(definition.source_tool)
      setTargetTool(definition.target_tool)
      setAccumulate(definition.accumulate)
      setParamMapping(definition.param_mapping?.length > 0 ? definition.param_mapping : [])
      setIsTemplate(definition.is_template || false)
      setTemplateArgs({})
    } else if (selectedTemplate) {
      setName('')
      setDescription('')
      setDirection('response')
      setSourceTool('')
      setTargetTool('')
      setAccumulate(selectedTemplate.accumulate || false)
      setParamMapping(selectedTemplate.param_mapping?.length > 0 ? [...selectedTemplate.param_mapping] : [])
      setIsTemplate(false)
      setTemplateArgs({})
    } else {
      setName('')
      setDescription('')
      setDirection('response')
      setSourceTool('')
      setTargetTool('')
      setAccumulate(false)
      setParamMapping([])
      setIsTemplate(false)
      setTemplateArgs({})
    }
  }, [definition, selectedTemplate, open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)

    const validParamMapping = paramMapping.filter((pm) => pm.from && pm.to)

    let newDef: TransformerDef

    if (isFromTemplate && selectedTemplate) {
      newDef = {
        name,
        description: description || undefined,
        direction: 'response',
        source_tool: '',
        target_tool: '',
        accumulate: selectedTemplate.accumulate || false,
        param_mapping: validParamMapping,
        builtin: false,
        template_ref: selectedTemplate.name,
        template_args: templateArgs,
      }
    } else if (isTemplate) {
      newDef = {
        name,
        description: description || undefined,
        direction: direction as 'request' | 'response',
        source_tool: sourceTool,
        target_tool: targetTool,
        accumulate,
        param_mapping: validParamMapping,
        builtin: false,
        is_template: true,
      }
    } else {
      newDef = {
        name,
        description: description || undefined,
        direction,
        source_tool: sourceTool,
        target_tool: targetTool,
        accumulate,
        param_mapping: validParamMapping,
        builtin: false,
      }
    }

    let success: boolean
    if (isEditing) {
      success = await updateDefinition(apiUrl, definition.name, newDef)
    } else {
      success = await createDefinition(apiUrl, newDef)
    }

    setSaving(false)
    if (success) {
      onClose()
    }
  }

  const updateParamMapping = (index: number, updates: Partial<ParamMapping>) => {
    setParamMapping((prev) => {
      const newMappings = [...prev]
      newMappings[index] = { ...newMappings[index], ...updates }
      return newMappings
    })
  }

  const addParamMapping = () => {
    setParamMapping((prev) => [...prev, createEmptyParamMapping()])
  }

  const removeParamMapping = (index: number) => {
    setParamMapping((prev) => prev.filter((_, i) => i !== index))
  }

  const updateTemplateArg = (key: string, value: string) => {
    setTemplateArgs((prev) => ({ ...prev, [key]: value }))
  }

  const getDialogTitle = () => {
    if (isEditing) return 'Edit Transformer'
    if (isFromTemplate) return `Create from Template: ${selectedTemplate?.name}`
    return 'Add Transformer'
  }

  return (
    <ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <ResizableDialogContent
        defaultWidth={550}
        defaultHeight={isFromTemplate ? 450 : 600}
        minWidth={450}
        minHeight={400}
      >
        <ResizableDialogHeader>
          <ResizableDialogTitle>{getDialogTitle()}</ResizableDialogTitle>
        </ResizableDialogHeader>
        <ResizableDialogBody>
          <form id="transformer-form" onSubmit={handleSubmit} className="space-y-4">
            {isFromTemplate ? (
              <>
                <Input
                  label="Name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="transformer_name"
                  required
                />

                <Input
                  label="Description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional description"
                />

                <div className="rounded-md border bg-muted/30 p-3">
                  <label className="mb-2 block text-sm font-medium">Template Parameters</label>
                  <div className="space-y-3">
                    {templateParams.map((param) => (
                      param === 'direction' ? (
                        <div key={param}>
                          <label className="mb-1.5 block text-sm font-medium">direction</label>
                          <Select
                            value={templateArgs[param] || 'response'}
                            onChange={(e) => updateTemplateArg(param, e.target.value)}
                            className="w-full"
                          >
                            <option value="response">response</option>
                            <option value="request">request</option>
                          </Select>
                        </div>
                      ) : (
                        <Input
                          key={param}
                          label={param}
                          value={templateArgs[param] || ''}
                          onChange={(e) => updateTemplateArg(param, e.target.value)}
                          placeholder={`Enter ${param}`}
                          required
                        />
                      )
                    ))}
                  </div>
                </div>
              </>
            ) : (
              <>
                <div className="grid grid-cols-2 gap-4">
                  <Input
                    label="Name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="transformer_name"
                    required
                    disabled={isEditing}
                  />
                  <div>
                    <label className="mb-1.5 block text-sm font-medium">Direction</label>
                    <Select
                      value={direction}
                      onChange={(e) => setDirection(e.target.value as 'request' | 'response')}
                      className="w-full"
                    >
                      <option value="response">Response</option>
                      <option value="request">Request</option>
                    </Select>
                  </div>
                </div>

                <Input
                  label="Description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional description"
                />

                <div className="grid grid-cols-2 gap-4">
                  <Input
                    label={isTemplate ? "Source Tool (use {{source}})" : "Source Tool"}
                    value={sourceTool}
                    onChange={(e) => setSourceTool(e.target.value)}
                    placeholder={isTemplate ? "{{source}}" : "source_tool_name"}
                    required
                  />
                  <Input
                    label={isTemplate ? "Target Tool (use {{target}})" : "Target Tool"}
                    value={targetTool}
                    onChange={(e) => setTargetTool(e.target.value)}
                    placeholder={isTemplate ? "{{target}}" : "target_tool_name"}
                    required
                  />
                </div>

                {!isEditing && (
                  <Checkbox
                    label="Create as Template (use {{param}} placeholders)"
                    checked={isTemplate}
                    onChange={setIsTemplate}
                  />
                )}

                {isTemplate && (
                  <p className="text-xs text-muted-foreground">
                    Use {"{{direction}}"}, {"{{source}}"}, {"{{target}}"} as placeholders in the fields above.
                  </p>
                )}

                {direction === 'response' && !isTemplate && (
                  <Checkbox
                    label="Accumulate (for streaming responses)"
                    checked={accumulate}
                    onChange={setAccumulate}
                  />
                )}

                {!isTemplate && (
                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <label className="text-sm font-medium">Parameter Mappings</label>
                      <button
                        type="button"
                        onClick={addParamMapping}
                        className="flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent"
                      >
                        <Plus className="h-3 w-3" /> Add Mapping
                      </button>
                    </div>
                    {paramMapping.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        No parameter mappings. Click "Add Mapping" to define how parameters are transformed.
                      </p>
                    ) : (
                      <div className="space-y-2">
                        {paramMapping.map((pm, index) => (
                          <div key={index} className="rounded-md border bg-muted/30 p-3">
                            <div className="flex items-start gap-2">
                              <div className="flex-1 space-y-2">
                                <div className="grid grid-cols-2 gap-2">
                                  <input
                                    type="text"
                                    value={pm.from}
                                    onChange={(e) => updateParamMapping(index, { from: e.target.value })}
                                    className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                                    placeholder="from (e.g., content)"
                                  />
                                  <input
                                    type="text"
                                    value={pm.to}
                                    onChange={(e) => updateParamMapping(index, { to: e.target.value })}
                                    className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                                    placeholder="to (e.g., plan)"
                                  />
                                </div>
                                <input
                                  type="text"
                                  value={pm.transform || ''}
                                  onChange={(e) => updateParamMapping(index, { transform: e.target.value })}
                                  className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                                  placeholder="transform (optional, e.g., string_to_array)"
                                />
                              </div>
                              <button
                                type="button"
                                onClick={() => removeParamMapping(index)}
                                className="rounded p-1.5 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </>
            )}

            {error && <p className="text-sm text-red-500">{error}</p>}
          </form>
        </ResizableDialogBody>
        <ResizableDialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="transformer-form" disabled={saving}>
            {saving ? 'Saving...' : isEditing ? 'Update' : 'Create'}
          </Button>
        </ResizableDialogFooter>
      </ResizableDialogContent>
    </ResizableDialog>
  )
}
