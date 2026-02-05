import { useState, useEffect } from 'react'
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogFooter,
  ResizableDialogTitle,
} from '../ui/resizable-dialog'
import { Input, Checkbox, Button } from '../ui/dialog'
import { useGatewayStore, GatewayConfig } from '../../store/gateway-store'
import { useConnectionStore } from '../../store/connection-store'

interface GatewayFormProps {
  open: boolean
  onClose: () => void
  gateway?: GatewayConfig | null
}

export function GatewayForm({ open, onClose, gateway }: GatewayFormProps) {
  const { apiUrl } = useConnectionStore()
  const { createGateway, updateGateway, error } = useGatewayStore()
  const [loading, setLoading] = useState(false)

  const [form, setForm] = useState<GatewayConfig>({
    name: '',
    path: '',
    upstream: '',
    type: '',
    timeout: 300,
    enabled: true,
  })

  useEffect(() => {
    if (gateway) {
      setForm(gateway)
    } else {
      setForm({
        name: '',
        path: '',
        upstream: '',
        type: '',
        timeout: 300,
        enabled: true,
      })
    }
  }, [gateway, open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)

    let success: boolean
    if (gateway) {
      success = await updateGateway(apiUrl, gateway.name, form)
    } else {
      success = await createGateway(apiUrl, form)
    }

    setLoading(false)
    if (success) {
      onClose()
    }
  }

  const isEdit = !!gateway

  return (
    <ResizableDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <ResizableDialogContent
        defaultWidth={450}
        defaultHeight={480}
        minWidth={350}
        minHeight={400}
      >
        <ResizableDialogHeader>
          <ResizableDialogTitle>{isEdit ? 'Edit Gateway' : 'Add Gateway'}</ResizableDialogTitle>
        </ResizableDialogHeader>
        <ResizableDialogBody>
          <form id="gateway-form" onSubmit={handleSubmit} className="space-y-4">
            <Input
              label="Name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="e.g., anthropic"
              required
              disabled={isEdit}
            />

            <Input
              label="Path"
              value={form.path}
              onChange={(e) => setForm({ ...form, path: e.target.value })}
              placeholder="e.g., /v1"
              required
            />

            <Input
              label="Upstream URL"
              value={form.upstream}
              onChange={(e) => setForm({ ...form, upstream: e.target.value })}
              placeholder="e.g., https://api.anthropic.com"
              required
            />

            <Input
              label="Upstream Type"
              value={form.type}
              onChange={(e) => setForm({ ...form, type: e.target.value })}
              placeholder="e.g., warp, kiro (generates $u_{type} tag)"
            />

            <Input
              label="Timeout (seconds)"
              type="number"
              value={form.timeout}
              onChange={(e) => setForm({ ...form, timeout: parseInt(e.target.value) || 300 })}
              min={1}
              required
            />

            <Checkbox
              label="Enabled"
              checked={form.enabled}
              onChange={(checked) => setForm({ ...form, enabled: checked })}
            />

            {error && <p className="text-sm text-red-500">{error}</p>}
          </form>
        </ResizableDialogBody>
        <ResizableDialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="gateway-form" disabled={loading}>
            {loading ? 'Saving...' : isEdit ? 'Update' : 'Create'}
          </Button>
        </ResizableDialogFooter>
      </ResizableDialogContent>
    </ResizableDialog>
  )
}
