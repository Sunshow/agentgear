import { ScrollArea } from '../ui/scroll-area'
import { cn } from '../../lib/utils'

interface PageContainerProps {
  children: React.ReactNode
  className?: string
}

export function PageContainer({ children, className }: PageContainerProps) {
  return (
    <ScrollArea className="h-full">
      <div className={cn('flex flex-col p-6', className)}>{children}</div>
    </ScrollArea>
  )
}
