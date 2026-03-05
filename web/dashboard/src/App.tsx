import { useState, useCallback } from 'react'
import {
  LayoutDashboard, TrendingUp, History, BarChart3,
  Cpu, Activity, ScrollText, MessageSquare, Settings,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { DeckView } from '@/views/deck'
import { ChatView } from '@/views/chat'
import { ProvidersView } from '@/views/providers'
import { DecisionsView } from '@/views/decisions'
import { LogsView } from '@/views/logs'
import './index.css'

type View = 'deck' | 'equity' | 'decisions' | 'analytics' | 'providers' | 'diagnostics' | 'logs' | 'chat' | 'config'

const NAV_GROUPS = [
  {
    label: 'CORE',
    items: [
      { id: 'deck' as View, icon: LayoutDashboard, label: 'Control Deck' },
      { id: 'chat' as View, icon: MessageSquare, label: 'Chat' },
      { id: 'equity' as View, icon: TrendingUp, label: 'Equity' },
      { id: 'decisions' as View, icon: History, label: 'Decisions' },
      { id: 'analytics' as View, icon: BarChart3, label: 'Analytics' },
    ],
  },
  {
    label: 'SYSTEM',
    items: [
      { id: 'providers' as View, icon: Cpu, label: 'Providers' },
      { id: 'diagnostics' as View, icon: Activity, label: 'Diagnostics' },
      { id: 'logs' as View, icon: ScrollText, label: 'Logs' },
      { id: 'config' as View, icon: Settings, label: 'Config' },
    ],
  },
]

export default function App() {
  const [view, setView] = useState<View>('deck')
  const [sseSnapshot, setSseSnapshot] = useState<unknown>(null)

  const onSnapshot = useCallback((data: unknown) => setSseSnapshot(data), [])

  return (
    <div className="flex h-screen overflow-hidden">
      {/* ── Sidebar ── */}
      <aside className="w-[220px] min-w-[220px] h-screen flex flex-col bg-surface border-r border-border sticky top-0 shrink-0">
        <div className="px-4 py-4 border-b border-border">
          <div className="font-mono font-semibold text-[15px] text-amber tracking-wide">PhantomClaw</div>
          <div className="font-mono text-[11px] text-muted-2 mt-0.5">v4.2.1</div>
        </div>

        <nav className="flex-1 overflow-y-auto py-2">
          {NAV_GROUPS.map((group) => (
            <div key={group.label} className="mb-1">
              <div className="px-4 py-2 text-[10px] font-semibold tracking-[.12em] uppercase text-muted-2">
                {group.label}
              </div>
              {group.items.map((item) => (
                <button
                  key={item.id}
                  onClick={() => setView(item.id)}
                  className={cn(
                    'w-full flex items-center gap-2.5 px-4 py-2 text-[13px] font-medium transition-all border-l-2 border-transparent',
                    view === item.id
                      ? 'text-amber border-l-amber bg-amber/10'
                      : 'text-muted hover:text-[#e2e8f0] hover:bg-surface-2'
                  )}
                >
                  <item.icon className="w-4 h-4 shrink-0" />
                  <span>{item.label}</span>
                </button>
              ))}
            </div>
          ))}
        </nav>

        <div className="px-4 py-3 border-t border-border flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-ok animate-pulse" />
          <span className="font-mono text-[11px] text-muted">Connected</span>
        </div>
      </aside>

      {/* ── Main ── */}
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <div className="flex-1 overflow-y-auto p-5">
          {view === 'deck' && <DeckView snapshot={sseSnapshot} onSnapshot={onSnapshot} />}
          {view === 'chat' && <ChatView />}
          {view === 'providers' && <ProvidersView />}
          {view === 'decisions' && <DecisionsView />}
          {view === 'logs' && <LogsView />}
          {view === 'equity' && <PlaceholderView name="Equity" />}
          {view === 'analytics' && <PlaceholderView name="Analytics" />}
          {view === 'diagnostics' && <PlaceholderView name="Diagnostics" />}
          {view === 'config' && <PlaceholderView name="Config" />}
        </div>
      </main>
    </div>
  )
}

function PlaceholderView({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center h-64 text-muted text-sm">
      {name} view — coming in Phase 2
    </div>
  )
}
