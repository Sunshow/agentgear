import { useState, useEffect } from 'react'
import { ParsedData, SystemPrompt, SystemReminder, ToolDefinition } from '../../store/connection-store'
import { useTransformerStore } from '../../store/transformer-store'
import { useMappingStore } from '../../store/mapping-store'
import { useConnectionStore } from '../../store/connection-store'
import { MappingForm } from '../mappings/MappingForm'
import { Button } from '../ui/dialog'

interface ProtocolViewProps {
  parsedData?: ParsedData
}

function copyToClipboard(data: unknown) {
  const text = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  navigator.clipboard.writeText(text || '')
}

function getLineCount(text: string): number {
  return text.split('\n').length
}

export function ProtocolView({ parsedData }: ProtocolViewProps) {
  if (!parsedData || !parsedData.anthropic) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">No protocol data available</p>
      </div>
    )
  }

  const { anthropic } = parsedData

  return (
    <div className="space-y-4">
      <Section title="Model Info">
        <div className="space-y-1 text-sm">
          <div className="flex gap-2">
            <span className="text-muted-foreground">Model:</span>
            <span className="font-mono">{anthropic.model || 'N/A'}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-muted-foreground">Max Tokens:</span>
            <span className="font-mono">{anthropic.max_tokens || 'N/A'}</span>
          </div>
        </div>
      </Section>

      <Section title={`System Prompts (${anthropic.system_prompts?.length || 0})`}>
        {anthropic.system_prompts?.length ? (
          <div className="space-y-2">
            {anthropic.system_prompts.map((prompt, i) => (
              <SystemPromptItem key={i} prompt={prompt} index={i} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No system prompts</p>
        )}
      </Section>

      <Section title={`System Reminders (${anthropic.system_reminders?.length || 0})`}>
        {anthropic.system_reminders?.length ? (
          <div className="space-y-2">
            {anthropic.system_reminders.map((reminder, i) => (
              <SystemReminderItem key={i} reminder={reminder} index={i} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No system reminders</p>
        )}
      </Section>

      <Section title={`Tools (${anthropic.tools?.length || 0})`}>
        {anthropic.tools?.length ? (
          <div className="space-y-2">
            {anthropic.tools.map((tool, i) => (
              <ToolItem key={i} tool={tool} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No tools</p>
        )}
      </Section>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-xs font-medium uppercase text-muted-foreground">{title}</h3>
      {children}
    </div>
  )
}

function SystemPromptItem({ prompt, index }: { prompt: SystemPrompt; index: number }) {
  const [expanded, setExpanded] = useState(false)
  const previewLength = 200
  const needsExpand = prompt.text.length > previewLength
  const displayText = expanded || !needsExpand ? prompt.text : prompt.text.slice(0, previewLength) + '...'
  
  const naturalLineCount = getLineCount(displayText)
  const estimatedLines = Math.ceil(displayText.length / 80)
  const lineCount = Math.max(naturalLineCount, expanded ? estimatedLines : 1)

  return (
    <div className="rounded border p-2 overflow-hidden">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          #{index + 1} {prompt.type}
          {prompt.cache_control && (
            <span className="ml-2 rounded bg-blue-100 px-1 text-blue-700 dark:bg-blue-900 dark:text-blue-300">
              cache: {prompt.cache_control}
            </span>
          )}
        </span>
        <div className="flex gap-1">
          <Button
            variant="outline"
            onClick={() => copyToClipboard(prompt.text)}
            className="h-5 px-2 text-xs"
          >
            Copy
          </Button>
          {needsExpand && (
            <Button
              variant="outline"
              onClick={() => setExpanded(!expanded)}
              className="h-5 px-2 text-xs"
            >
              {expanded ? 'Collapse' : 'Expand'}
            </Button>
          )}
        </div>
      </div>
      <textarea
        readOnly
        value={displayText}
        className="w-full resize-none rounded bg-muted p-2 text-xs font-mono focus:outline-none"
        rows={Math.min(lineCount, 15)}
      />
    </div>
  )
}

function SystemReminderItem({ reminder, index }: { reminder: SystemReminder; index: number }) {
  const [expanded, setExpanded] = useState(false)
  const previewLength = 300
  const needsExpand = reminder.raw_text.length > previewLength
  const displayText = expanded || !needsExpand ? reminder.raw_text : reminder.raw_text.slice(0, previewLength) + '...'
  
  const naturalLineCount = getLineCount(displayText)
  const estimatedLines = Math.ceil(displayText.length / 80)
  const lineCount = Math.max(naturalLineCount, expanded ? estimatedLines : 1)

  return (
    <div className="rounded border p-2 overflow-hidden">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">#{index + 1}</span>
        <div className="flex gap-1">
          <Button
            variant="outline"
            onClick={() => copyToClipboard(reminder.raw_text)}
            className="h-5 px-2 text-xs"
          >
            Copy
          </Button>
          {needsExpand && (
            <Button
              variant="outline"
              onClick={() => setExpanded(!expanded)}
              className="h-5 px-2 text-xs"
            >
              {expanded ? 'Collapse' : 'Expand'}
            </Button>
          )}
        </div>
      </div>
      {reminder.parsed_info && Object.keys(reminder.parsed_info).length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {Object.entries(reminder.parsed_info).map(([key, value]) => (
            <span
              key={key}
              className="rounded bg-yellow-100 px-1.5 py-0.5 text-xs text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
            >
              {key}: {value}
            </span>
          ))}
        </div>
      )}
      <textarea
        readOnly
        value={displayText}
        className="w-full resize-none rounded bg-muted p-2 text-xs font-mono focus:outline-none"
        rows={Math.min(lineCount, 15)}
      />
    </div>
  )
}

function ToolItem({ tool }: { tool: ToolDefinition }) {
  const [expanded, setExpanded] = useState(false)
  const [showMappingForm, setShowMappingForm] = useState(false)
  const { definitions, fetchDefinitions } = useTransformerStore()
  const { setEditingMapping } = useMappingStore()
  const { apiUrl } = useConnectionStore()
  const schemaText = tool.input_schema ? JSON.stringify(tool.input_schema, null, 2) : ''
  const schemaLineCount = schemaText ? getLineCount(schemaText) : 0

  useEffect(() => {
    fetchDefinitions(apiUrl)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiUrl])

  const copyInfo = () => {
    const info = tool.description ? `${tool.name}\n${tool.description}` : tool.name
    copyToClipboard(info)
  }

  const matchingTransformer = definitions.find((d) => d.source_tool === tool.name)

  const handleAddMapping = () => {
    setEditingMapping(null)
    setShowMappingForm(true)
  }

  return (
    <div className="rounded border p-2 overflow-hidden">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{tool.name}</span>
        <div className="flex gap-1">
          <Button
            variant="outline"
            onClick={copyInfo}
            className="h-5 px-2 text-xs"
          >
            Copy Info
          </Button>
          {tool.input_schema && (
            <>
              <Button
                variant="outline"
                onClick={() => copyToClipboard(tool.input_schema)}
                className="h-5 px-2 text-xs"
              >
                Copy Schema
              </Button>
              <Button
                variant="outline"
                onClick={() => setExpanded(!expanded)}
                className="h-5 px-2 text-xs"
              >
                {expanded ? 'Hide Schema' : 'Show Schema'}
              </Button>
            </>
          )}
          {matchingTransformer && (
            <Button
              variant="outline"
              onClick={handleAddMapping}
              className="h-5 px-2 text-xs bg-green-500/10 hover:bg-green-500/20 text-green-600 dark:text-green-400"
              title={`Create mapping for transformer: ${matchingTransformer.name}`}
            >
              + Mapping
            </Button>
          )}
        </div>
      </div>
      {tool.description && (
        <p className="mt-1 text-xs text-muted-foreground line-clamp-2">{tool.description}</p>
      )}
      {expanded && tool.input_schema && (
        <textarea
          readOnly
          value={schemaText}
          className="mt-2 w-full resize-none rounded bg-muted p-2 text-xs font-mono focus:outline-none"
          rows={Math.min(schemaLineCount, 15)}
        />
      )}
      <MappingForm
        open={showMappingForm}
        onClose={() => setShowMappingForm(false)}
        mapping={null}
      />
    </div>
  )
}
