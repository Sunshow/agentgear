import { X } from 'lucide-react'
import { cn } from '../../lib/utils'

interface SheetProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  width?: string
}

export function Sheet({ open, onClose, title, children, width = 'w-96' }: SheetProps) {
  return (
    <>
      <div
        className={cn(
          'fixed inset-0 z-40 bg-black/50 transition-opacity duration-300',
          open ? 'opacity-100' : 'pointer-events-none opacity-0'
        )}
        onClick={onClose}
      />
      <div
        className={cn(
          'fixed right-0 top-0 z-50 h-full border-l bg-[var(--background)] shadow-lg transition-transform duration-300',
          width,
          open ? 'translate-x-0' : 'translate-x-full'
        )}
      >
        <div className="flex h-12 items-center justify-between border-b px-4">
          <h2 className="text-sm font-semibold">{title}</h2>
          <button
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="h-[calc(100%-3rem)] overflow-auto">{children}</div>
      </div>
    </>
  )
}
