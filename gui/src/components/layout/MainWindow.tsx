import { useState } from 'react'
import { ResizablePanelGroup, ResizablePanel, ResizableHandle } from '../ui/resizable'
import { ConnectionList } from '../connections/ConnectionList'
import { Inspector } from '../inspector/Inspector'
import { GatewayList } from '../gateways/GatewayList'
import { TaggingList } from '../tagging/TaggingList'
import { TransformerList } from '../transformers/TransformerList'
import { MappingList } from '../mappings/MappingList'
import { Header } from './Header'
import { Sidebar } from './Sidebar'

export function MainWindow() {
  const [activeView, setActiveView] = useState<'connections' | 'gateways' | 'tagging' | 'transformers' | 'mappings'>('connections')

  return (
    <div className="flex h-screen w-full flex-col overflow-hidden bg-background">
      <Header />

      <div className="flex flex-1 overflow-hidden">
        <Sidebar activeView={activeView} onViewChange={setActiveView} />

        {activeView === 'connections' ? (
          <ResizablePanelGroup direction="horizontal" className="flex-1">
            <ResizablePanel defaultSize={30} minSize={20}>
              <ConnectionList />
            </ResizablePanel>

            <ResizableHandle />

            <ResizablePanel defaultSize={70} minSize={30}>
              <Inspector />
            </ResizablePanel>
          </ResizablePanelGroup>
        ) : activeView === 'gateways' ? (
          <div className="flex-1">
            <GatewayList />
          </div>
        ) : activeView === 'tagging' ? (
          <div className="flex-1">
            <TaggingList />
          </div>
        ) : activeView === 'transformers' ? (
          <div className="flex-1">
            <TransformerList />
          </div>
        ) : (
          <div className="flex-1">
            <MappingList />
          </div>
        )}
      </div>
    </div>
  )
}
