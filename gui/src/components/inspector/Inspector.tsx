import { useConnectionStore } from '../../store/connection-store'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../ui/tabs'
import { PageContainer } from '../layout/PageContainer'
import { Button } from '../ui/dialog'
import { ProtocolView } from './ProtocolView'
import { TagGroup } from '../ui/tag-group'

export function Inspector() {
  const { selectedId, connections } = useConnectionStore()
  const connection = connections.find((c) => c.id === selectedId)

  if (!connection) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">Select a connection to inspect</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <h2 className="text-sm font-medium">
          {connection.method} {connection.path}
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          Session: {connection.session_id} | Sequence: #{connection.sequence}
        </p>
      </div>

      <Tabs defaultValue="request" className="flex flex-1 flex-col overflow-hidden">
        <TabsList className="mx-3 mt-2 w-fit">
          <TabsTrigger value="request">Request</TabsTrigger>
          <TabsTrigger value="response">Response</TabsTrigger>
          <TabsTrigger value="protocol">Protocol</TabsTrigger>
          <TabsTrigger value="tags">Tags & Mappings</TabsTrigger>
        </TabsList>

        <TabsContent value="request" className="flex-1 overflow-hidden">
          <PageContainer className="p-3">
            <div className="space-y-4">
              <Section title="Headers" onCopy={() => copyToClipboard(connection.request_headers)}>
                <JsonView data={connection.request_headers} />
              </Section>
              <Section title="Body" onCopy={() => copyToClipboard(tryParseJson(connection.request_body))}>
                <JsonView data={tryParseJson(connection.request_body)} />
              </Section>
            </div>
          </PageContainer>
        </TabsContent>

        <TabsContent value="response" className="flex-1 overflow-hidden">
          <PageContainer className="p-3">
            <div className="space-y-4">
              <Section title="Status">
                <p className="text-sm">{connection.response_status || 'N/A'}</p>
              </Section>
              <Section title="Headers" onCopy={() => copyToClipboard(connection.response_headers)}>
                <JsonView data={connection.response_headers} />
              </Section>
              <Section title="Body" onCopy={() => copyToClipboard(tryParseJson(connection.response_body))}>
                <JsonView data={tryParseJson(connection.response_body)} />
              </Section>
            </div>
          </PageContainer>
        </TabsContent>

        <TabsContent value="protocol" className="flex-1 overflow-hidden">
          <PageContainer className="p-3">
            <ProtocolView parsedData={connection.parsed_data} />
          </PageContainer>
        </TabsContent>

        <TabsContent value="tags" className="flex-1 overflow-hidden">
          <PageContainer className="p-3">
            <div className="space-y-4">
              <Section title={`Tags (${connection.tags?.length || 0})`}>
                {connection.tags && connection.tags.length > 0 ? (
                  <TagGroup tags={connection.tags} showLabels={true} size="sm" />
                ) : (
                  <p className="text-sm text-muted-foreground">No tags</p>
                )}
              </Section>
              <Section title={`Applied Mappings (${connection.applied_transformers?.length || 0})`}>
                {connection.applied_transformers && connection.applied_transformers.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {connection.applied_transformers.map((name, idx) => (
                      <span
                        key={idx}
                        className="inline-flex items-center rounded-md bg-blue-500/10 px-2 py-1 text-xs font-medium text-blue-500"
                      >
                        {name}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">No mappings applied</p>
                )}
              </Section>
            </div>
          </PageContainer>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function Section({ title, children, onCopy }: { title: string; children: React.ReactNode; onCopy?: () => void }) {
  return (
    <div>
      <div className="mb-2 flex items-center gap-2">
        <h3 className="text-xs font-medium uppercase text-muted-foreground">{title}</h3>
        {onCopy && (
          <Button
            variant="outline"
            onClick={onCopy}
            className="h-6 px-2 text-xs transition-all hover:bg-primary hover:text-primary-foreground active:scale-95"
          >
            Copy
          </Button>
        )}
      </div>
      {children}
    </div>
  )
}

function copyToClipboard(data: unknown) {
  const text = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  navigator.clipboard.writeText(text || '')
}

function JsonView({ data }: { data: unknown }) {
  if (!data) {
    return <p className="text-sm text-muted-foreground">Empty</p>
  }

  const content = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  const lineCount = content.split('\n').length

  return (
    <textarea
      readOnly
      value={content}
      className="w-full resize-none rounded bg-muted p-2 text-xs font-mono focus:outline-none"
      rows={Math.min(lineCount, 25)}
    />
  )
}

function tryParseJson(data: unknown): unknown {
  if (typeof data === 'string') {
    try {
      return JSON.parse(data)
    } catch {
      return data
    }
  }
  if (data instanceof Uint8Array) {
    try {
      const str = new TextDecoder().decode(data)
      return JSON.parse(str)
    } catch {
      return new TextDecoder().decode(data)
    }
  }
  return data
}
