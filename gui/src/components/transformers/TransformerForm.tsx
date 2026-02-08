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
import { useTransformerStore, TransformerDef, ParamMapping, HeaderInjection } from '../../store/transformer-store'
import { useConnectionStore } from '../../store/connection-store'

interface TransformerFormProps {
  open: boolean
  onClose: () => void
  definition: TransformerDef | null
}

function createEmptyParamMapping(): ParamMapping {
  return { from: '', to: '', transform: '' }
}

function createEmptyHeaderInjection(): HeaderInjection {
  return { key: '', value: '' }
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
  const [type, setType] = useState<'tool' | 'message_inject' | 'error_transform' | 'header_inject' | 'compress'>('tool')
  const [direction, setDirection] = useState<'request' | 'response'>('response')
  
  // Tool transform fields
  const [sourceTool, setSourceTool] = useState('')
  const [targetTool, setTargetTool] = useState('')
  const [accumulate, setAccumulate] = useState(false)
  const [paramMapping, setParamMapping] = useState<ParamMapping[]>([])
  
  // Message inject fields
  const [injectText, setInjectText] = useState('')
  const [injectFormat, setInjectFormat] = useState<'system-reminder' | 'plain'>('system-reminder')
  
  // Error transform fields
  const [errorCode, setErrorCode] = useState('')
  const [errorMessage, setErrorMessage] = useState('')
  const [requestSizeThreshold, setRequestSizeThreshold] = useState(500000)
  const [contextTokenLimit, setContextTokenLimit] = useState(200000)
  const [contextThresholdRatio, setContextThresholdRatio] = useState(0.85)
  const [tokenEstimateRatio, setTokenEstimateRatio] = useState(3.5)
  
  // Compress fields
  const [compressTarget, setCompressTarget] = useState('same')
  const [compressModel, setCompressModel] = useState('claude-3-5-sonnet-20241022')
  const [compressSystemPrompt, setCompressSystemPrompt] = useState('')
  const [compressUserPrompt, setCompressUserPrompt] = useState('')
  const [preserveBudget, setPreserveBudget] = useState(40000)
  const [summaryBudget, setSummaryBudget] = useState(4000)
  const [autoRetry, setAutoRetry] = useState(true)
  const [maxRetries, setMaxRetries] = useState(1)
  
  // Common fields
  const [headerInjections, setHeaderInjections] = useState<HeaderInjection[]>([])
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
      setType(definition.type || 'tool')
      setDirection(definition.direction)
      
      // Tool fields
      setSourceTool(definition.source_tool || '')
      setTargetTool(definition.target_tool || '')
      setAccumulate(definition.accumulate || false)
      setParamMapping(definition.param_mapping?.length > 0 ? definition.param_mapping : [])
      
      // Message inject fields
      setInjectText(definition.inject_text || '')
      setInjectFormat(definition.inject_format || 'system-reminder')
      
      // Error transform fields
      setErrorCode(definition.error_code || '')
      setErrorMessage(definition.error_message || '')
      setRequestSizeThreshold(definition.request_size_threshold || 500000)
      setContextTokenLimit(definition.context_token_limit || 200000)
      setContextThresholdRatio(definition.context_threshold_ratio || 0.85)
      setTokenEstimateRatio(definition.token_estimate_ratio || 3.5)
      
      // Compress fields
      setCompressTarget(definition.compress_target || 'same')
      setCompressModel(definition.compress_model || 'claude-3-5-sonnet-20241022')
      setCompressSystemPrompt(definition.compress_system_prompt || '')
      setCompressUserPrompt(definition.compress_user_prompt || '')
      setPreserveBudget(definition.preserve_budget || 40000)
      setSummaryBudget(definition.summary_budget || 4000)
      setAutoRetry(definition.auto_retry !== undefined ? definition.auto_retry : true)
      setMaxRetries(definition.max_retries || 1)
      
      // Common fields
      setHeaderInjections(definition.header_injections && definition.header_injections.length > 0 ? definition.header_injections : [])
      setIsTemplate(definition.is_template || false)
      setTemplateArgs({})
    } else if (selectedTemplate) {
      setName('')
      setDescription('')
      setType('tool')
      setDirection('response')
      setSourceTool('')
      setTargetTool('')
      setAccumulate(selectedTemplate.accumulate || false)
      setParamMapping(selectedTemplate.param_mapping?.length > 0 ? [...selectedTemplate.param_mapping] : [])
      setInjectText('')
      setInjectFormat('system-reminder')
      setErrorCode('')
      setErrorMessage('')
      setRequestSizeThreshold(500000)
      setContextTokenLimit(200000)
      setContextThresholdRatio(0.85)
      setTokenEstimateRatio(3.5)
      setCompressTarget('same')
      setCompressModel('claude-3-5-sonnet-20241022')
      setCompressSystemPrompt('')
      setCompressUserPrompt('')
      setPreserveBudget(40000)
      setSummaryBudget(4000)
      setAutoRetry(true)
      setMaxRetries(1)
      setHeaderInjections([])
      setIsTemplate(false)
      setTemplateArgs({})
    } else {
      setName('')
      setDescription('')
      setType('tool')
      setDirection('response')
      setSourceTool('')
      setTargetTool('')
      setAccumulate(false)
      setParamMapping([])
      setInjectText('')
      setInjectFormat('system-reminder')
      setErrorCode('')
      setErrorMessage('')
      setRequestSizeThreshold(500000)
      setContextTokenLimit(200000)
      setContextThresholdRatio(0.85)
      setTokenEstimateRatio(3.5)
      setCompressTarget('same')
      setCompressModel('claude-3-5-sonnet-20241022')
      setCompressSystemPrompt('')
      setCompressUserPrompt('')
      setPreserveBudget(40000)
      setSummaryBudget(4000)
      setAutoRetry(true)
      setMaxRetries(1)
      setHeaderInjections([])
      setIsTemplate(false)
      setTemplateArgs({})
    }
  }, [definition, selectedTemplate, open])

  // Adjust direction defaults when type changes
  useEffect(() => {
    if (type === 'message_inject') {
      setDirection('request')
    } else if (type === 'error_transform') {
      setDirection('response')
    } else if (type === 'compress') {
      setDirection('request')
    } else if (type === 'header_inject' && !definition) {
      // Default to request for header_inject when creating new
      setDirection('request')
    }
  }, [type, definition])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)

    const validParamMapping = paramMapping.filter((pm) => pm.from && pm.to)
    const validHeaderInjections = headerInjections.filter((h) => h.key && h.value)

    let newDef: TransformerDef

    if (isFromTemplate && selectedTemplate) {
      newDef = {
        name,
        description: description || undefined,
        type: 'tool',
        direction: 'response',
        source_tool: '',
        target_tool: '',
        accumulate: selectedTemplate.accumulate || false,
        param_mapping: validParamMapping,
        header_injections: validHeaderInjections.length > 0 ? validHeaderInjections : undefined,
        builtin: false,
        template_ref: selectedTemplate.name,
        template_args: templateArgs,
      }
    } else if (isTemplate) {
      newDef = {
        name,
        description: description || undefined,
        type: 'tool',
        direction: direction as 'request' | 'response',
        source_tool: sourceTool,
        target_tool: targetTool,
        accumulate,
        param_mapping: validParamMapping,
        header_injections: validHeaderInjections.length > 0 ? validHeaderInjections : undefined,
        builtin: false,
        is_template: true,
      }
    } else {
      // Normal transformer creation based on type
      newDef = {
        name,
        description: description || undefined,
        type: type || 'tool',
        direction,
        accumulate: false,
        param_mapping: [],
        builtin: false,
      }

      // Add type-specific fields
      if (type === 'tool') {
        newDef.source_tool = sourceTool
        newDef.target_tool = targetTool
        newDef.accumulate = accumulate
        newDef.param_mapping = validParamMapping
      } else if (type === 'message_inject') {
        newDef.direction = 'request'
        newDef.inject_text = injectText
        newDef.inject_format = injectFormat
      } else if (type === 'error_transform') {
        newDef.direction = 'response'
        newDef.error_code = errorCode
        newDef.error_message = errorMessage
        newDef.request_size_threshold = requestSizeThreshold
        newDef.context_token_limit = contextTokenLimit
        newDef.context_threshold_ratio = contextThresholdRatio
        newDef.token_estimate_ratio = tokenEstimateRatio
      } else if (type === 'compress') {
        newDef.direction = 'request'
        newDef.compress_target = compressTarget
        newDef.compress_model = compressModel
        newDef.compress_system_prompt = compressSystemPrompt || undefined
        newDef.compress_user_prompt = compressUserPrompt || undefined
        newDef.context_token_limit = contextTokenLimit
        newDef.context_threshold_ratio = contextThresholdRatio
        newDef.token_estimate_ratio = tokenEstimateRatio
        newDef.preserve_budget = preserveBudget
        newDef.summary_budget = summaryBudget
        newDef.auto_retry = autoRetry
        newDef.max_retries = maxRetries
      } else if (type === 'header_inject') {
        // No tool-specific fields, only direction and header_injections
      }

      // Add header injections (common to all types)
      if (validHeaderInjections.length > 0) {
        newDef.header_injections = validHeaderInjections
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

  const updateHeaderInjection = (index: number, updates: Partial<HeaderInjection>) => {
    setHeaderInjections((prev) => {
      const newHeaders = [...prev]
      newHeaders[index] = { ...newHeaders[index], ...updates }
      return newHeaders
    })
  }

  const addHeaderInjection = () => {
    setHeaderInjections((prev) => [...prev, createEmptyHeaderInjection()])
  }

  const removeHeaderInjection = (index: number) => {
    setHeaderInjections((prev) => prev.filter((_, i) => i !== index))
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
                    <label className="mb-1.5 block text-sm font-medium">Type</label>
                    <Select
                      value={type}
                      onChange={(e) => setType(e.target.value as 'tool' | 'message_inject' | 'error_transform' | 'header_inject' | 'compress')}
                      className="w-full"
                      disabled={isEditing}
                    >
                      <option value="tool">Tool Transform</option>
                      <option value="message_inject">Message Inject</option>
                      <option value="error_transform">Error Transform</option>
                      <option value="header_inject">Header Inject</option>
                      <option value="compress">Compress</option>
                    </Select>
                  </div>
                </div>

                <Input
                  label="Description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Optional description"
                />

                <div>
                  <label className="mb-1.5 block text-sm font-medium">Direction</label>
                  <Select
                    value={direction}
                    onChange={(e) => setDirection(e.target.value as 'request' | 'response')}
                    className="w-full"
                    disabled={type === 'message_inject' || type === 'error_transform' || type === 'compress'}
                  >
                    <option value="request">Request</option>
                    <option value="response">Response</option>
                  </Select>
                  {type === 'message_inject' && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Message inject only supports request direction
                    </p>
                  )}
                  {type === 'error_transform' && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Error transform only supports response direction
                    </p>
                  )}
                  {type === 'compress' && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Compress only supports request direction
                    </p>
                  )}
                </div>

                {/* === Type: tool 专用字段 === */}
                {type === 'tool' && (
                  <>
                    <div className="grid grid-cols-2 gap-4">
                      <Input
                        label="Source Tool"
                        value={sourceTool}
                        onChange={(e) => setSourceTool(e.target.value)}
                        placeholder="source_tool_name (empty = match all)"
                        required={false}
                      />
                      <Input
                        label="Target Tool"
                        value={targetTool}
                        onChange={(e) => setTargetTool(e.target.value)}
                        placeholder="target_tool_name (empty = no transform)"
                        required={false}
                      />
                    </div>

                    {direction === 'response' && (
                      <Checkbox
                        label="Accumulate (for streaming responses)"
                        checked={accumulate}
                        onChange={setAccumulate}
                      />
                    )}

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
                  </>
                )}

                {/* === Type: message_inject 专用字段 === */}
                {type === 'message_inject' && (
                  <>
                    <div>
                      <label className="mb-1.5 block text-sm font-medium">Inject Text</label>
                      <textarea
                        value={injectText}
                        onChange={(e) => setInjectText(e.target.value)}
                        className="w-full min-h-[100px] rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                        placeholder="Text to inject into request messages. Supports {{tool}} placeholder."
                        required
                      />
                    </div>

                    <div>
                      <label className="mb-1.5 block text-sm font-medium">Inject Format</label>
                      <Select
                        value={injectFormat}
                        onChange={(e) => setInjectFormat(e.target.value as 'system-reminder' | 'plain')}
                        className="w-full"
                      >
                        <option value="system-reminder">System Reminder (wrapped in tags)</option>
                        <option value="plain">Plain Text</option>
                      </Select>
                    </div>
                  </>
                )}

                {/* === Type: error_transform 专用字段 === */}
                {type === 'error_transform' && (
                  <>
                    <div className="grid grid-cols-2 gap-4">
                      <Input
                        label="Error Code"
                        value={errorCode}
                        onChange={(e) => setErrorCode(e.target.value)}
                        placeholder="e.g., context_length_exceeded"
                        required
                      />
                      <Input
                        label="Request Size Threshold (bytes)"
                        type="number"
                        value={requestSizeThreshold.toString()}
                        onChange={(e) => setRequestSizeThreshold(Number(e.target.value))}
                        placeholder="500000"
                      />
                    </div>

                    <div>
                      <label className="mb-1.5 block text-sm font-medium">Error Message</label>
                      <textarea
                        value={errorMessage}
                        onChange={(e) => setErrorMessage(e.target.value)}
                        className="w-full min-h-[60px] rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                        placeholder="Error message to return"
                        required
                      />
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <Input
                        label="Context Token Limit"
                        type="number"
                        value={contextTokenLimit.toString()}
                        onChange={(e) => setContextTokenLimit(Number(e.target.value))}
                        placeholder="200000"
                      />
                      <Input
                        label="Context Threshold Ratio"
                        type="number"
                        step="0.01"
                        value={contextThresholdRatio.toString()}
                        onChange={(e) => setContextThresholdRatio(Number(e.target.value))}
                        placeholder="0.85"
                      />
                    </div>

                    <p className="text-xs text-muted-foreground">
                      Advanced fields (model_context_limits, token_estimate_ratio, param_conditions) can be configured via YAML.
                    </p>
                  </>
                )}

                {/* === Type: compress 专用字段 === */}
                {type === 'compress' && (
                  <>
                    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
                      <h3 className="text-sm font-medium">Trigger Conditions</h3>
                      <div className="grid grid-cols-2 gap-4">
                        <Input
                          label="Context Token Limit"
                          type="number"
                          value={contextTokenLimit.toString()}
                          onChange={(e) => setContextTokenLimit(Number(e.target.value))}
                          placeholder="200000"
                          required
                        />
                        <Input
                          label="Threshold Ratio"
                          type="number"
                          step="0.01"
                          value={contextThresholdRatio.toString()}
                          onChange={(e) => setContextThresholdRatio(Number(e.target.value))}
                          placeholder="0.7"
                          required
                        />
                      </div>
                      <Input
                        label="Token Estimate Ratio"
                        type="number"
                        step="0.1"
                        value={tokenEstimateRatio.toString()}
                        onChange={(e) => setTokenEstimateRatio(Number(e.target.value))}
                        placeholder="3.5"
                        required
                      />
                    </div>

                    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
                      <h3 className="text-sm font-medium">Compression Configuration</h3>
                      <Input
                        label="Compress Target"
                        value={compressTarget}
                        onChange={(e) => setCompressTarget(e.target.value)}
                        placeholder='same | gateway:name | url:https://...'
                        required
                      />
                      <Input
                        label="Compress Model"
                        value={compressModel}
                        onChange={(e) => setCompressModel(e.target.value)}
                        placeholder="claude-3-5-sonnet-20241022"
                        required
                      />
                      <div>
                        <label className="mb-1.5 block text-sm font-medium">Compress System Prompt (optional)</label>
                        <textarea
                          value={compressSystemPrompt}
                          onChange={(e) => setCompressSystemPrompt(e.target.value)}
                          className="w-full min-h-[100px] rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                          placeholder="Leave empty to use default 13-part summary guidelines"
                        />
                      </div>
                      <div>
                        <label className="mb-1.5 block text-sm font-medium">Compress User Prompt (optional)</label>
                        <textarea
                          value={compressUserPrompt}
                          onChange={(e) => setCompressUserPrompt(e.target.value)}
                          className="w-full min-h-[60px] rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                          placeholder="Leave empty to use default prompt"
                        />
                      </div>
                    </div>

                    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
                      <h3 className="text-sm font-medium">Message Splitting</h3>
                      <div className="grid grid-cols-2 gap-4">
                        <Input
                          label="Preserve Budget (tokens)"
                          type="number"
                          value={preserveBudget.toString()}
                          onChange={(e) => setPreserveBudget(Number(e.target.value))}
                          placeholder="40000"
                          required
                        />
                        <Input
                          label="Summary Budget (tokens)"
                          type="number"
                          value={summaryBudget.toString()}
                          onChange={(e) => setSummaryBudget(Number(e.target.value))}
                          placeholder="4000"
                          required
                        />
                      </div>
                    </div>

                    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
                      <h3 className="text-sm font-medium">Post-Compression</h3>
                      <Checkbox
                        label="Auto Retry (automatically retry original request after compression)"
                        checked={autoRetry}
                        onChange={setAutoRetry}
                      />
                      <Input
                        label="Max Retries"
                        type="number"
                        value={maxRetries.toString()}
                        onChange={(e) => setMaxRetries(Number(e.target.value))}
                        placeholder="1"
                        required
                      />
                    </div>
                  </>
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

                {!isTemplate && (
                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <label className="text-sm font-medium">Header Injections</label>
                      <button
                        type="button"
                        onClick={addHeaderInjection}
                        className="flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-accent"
                      >
                        <Plus className="h-3 w-3" /> Add Header
                      </button>
                    </div>
                    {headerInjections.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        No custom headers. Click "Add Header" to inject HTTP headers.
                      </p>
                    ) : (
                      <div className="space-y-2">
                        {headerInjections.map((header, index) => (
                          <div key={index} className="rounded-md border bg-muted/30 p-3">
                            <div className="flex items-start gap-2">
                              <div className="flex-1 grid grid-cols-2 gap-2">
                                <input
                                  type="text"
                                  value={header.key}
                                  onChange={(e) => updateHeaderInjection(index, { key: e.target.value })}
                                  className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                                  placeholder="Header name (e.g., X-Custom-Agent)"
                                />
                                <input
                                  type="text"
                                  value={header.value}
                                  onChange={(e) => updateHeaderInjection(index, { value: e.target.value })}
                                  className="h-9 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                                  placeholder="Header value (supports {{placeholders}})"
                                />
                              </div>
                              <button
                                type="button"
                                onClick={() => removeHeaderInjection(index)}
                                className="rounded p-1.5 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                    <p className="mt-1 text-xs text-muted-foreground">
                      Supported placeholders: {"{{session_id}}"}, {"{{request_id}}"}, {"{{gateway}}"}
                    </p>
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
