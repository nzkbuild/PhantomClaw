import { useState, useCallback, useEffect } from 'react'
import {
  LayoutDashboard, TrendingUp, History, BarChart3,
  Cpu, Activity, ScrollText, MessageSquare, Settings,
  Gauge, FolderOpen, ShieldAlert, Clock, Zap, Command, Palette,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { DeckView } from '@/views/deck'
import { ChatView } from '@/views/chat'
import { ProvidersView } from '@/views/providers'
import { DecisionsView } from '@/views/decisions'
import { LogsView } from '@/views/logs'
import { UsageView } from '@/views/usage'
import { ConfigView } from '@/views/config'
import { SessionsView } from '@/views/sessions'
import { RiskView } from '@/views/risk'
import { CronView } from '@/views/cron'
import { FeedView } from '@/views/feed'
import { EquityView } from '@/views/equity'
import { AnalyticsView } from '@/views/analytics'
import { DiagnosticsView } from '@/views/diagnostics'
import { CommandPalette } from '@/components/command-palette'
import { NotificationCenter } from '@/components/notifications'
import { ThemeProvider, ThemeSwitcher } from '@/components/theme-provider'
import './index.css'

type View = 'deck' | 'equity' | 'decisions' | 'analytics' | 'providers' | 'diagnostics' | 'logs' | 'chat' | 'config' | 'usage' | 'sessions' | 'risk' | 'cron' | 'feed' | 'theme'

const NAV_GROUPS = [
  {
    label: 'CORE',
    items: [
      { id: 'deck' as View, icon: LayoutDashboard, label: 'Control Deck' },
      { id: 'chat' as View, icon: MessageSquare, label: 'Chat' },
      { id: 'feed' as View, icon: Zap, label: 'Live Feed' },
      { id: 'equity' as View, icon: TrendingUp, label: 'Equity' },
      { id: 'decisions' as View, icon: History, label: 'Decisions' },
      { id: 'analytics' as View, icon: BarChart3, label: 'Analytics' },
    ],
  },
  {
    label: 'DATA',
    items: [
      { id: 'usage' as View, icon: Gauge, label: 'Usage' },
      { id: 'sessions' as View, icon: FolderOpen, label: 'Sessions' },
      { id: 'risk' as View, icon: ShieldAlert, label: 'Risk' },
      { id: 'cron' as View, icon: Clock, label: 'Cron Jobs' },
    ],
  },
  {
    label: 'SYSTEM',
    items: [
      { id: 'providers' as View, icon: Cpu, label: 'Providers' },
      { id: 'diagnostics' as View, icon: Activity, label: 'Diagnostics' },
      { id: 'logs' as View, icon: ScrollText, label: 'Logs' },
      { id: 'config' as View, icon: Settings, label: 'Config' },
      { id: 'theme' as View, icon: Palette, label: 'Theme' },
    ],
  },
]

// Mobile bottom tab items
const MOBILE_TABS = [
  { id: 'deck' as View, icon: LayoutDashboard, label: 'Deck' },
  { id: 'chat' as View, icon: MessageSquare, label: 'Chat' },
  { id: 'feed' as View, icon: Zap, label: 'Feed' },
  { id: 'risk' as View, icon: ShieldAlert, label: 'Risk' },
  { id: 'config' as View, icon: Settings, label: 'More' },
]

export default function App() {
  const [view, setView] = useState<View>('deck')
  const [sseSnapshot, setSseSnapshot] = useState<unknown>(null)
  const [paletteOpen, setPaletteOpen] = useState(false)

  const onSnapshot = useCallback((data: unknown) => setSseSnapshot(data), [])

  // Ctrl+K handler
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setPaletteOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  return (
    <ThemeProvider>
      <div className="flex h-screen overflow-hidden">
        {/* ── Sidebar (desktop) ── */}
        <aside className="hidden md:flex w-[220px] min-w-[220px] h-screen flex-col bg-surface border-r border-border sticky top-0 shrink-0">
          <div className="px-4 py-4 border-b border-border">
            <div className="font-mono font-semibold text-[15px] text-amber tracking-wide">PhantomClaw</div>
            <div className="font-mono text-[11px] text-muted-2 mt-0.5">v4.2.1</div>
          </div>

          {/* Search trigger */}
          <button
            onClick={() => setPaletteOpen(true)}
            className="mx-3 mt-3 flex items-center gap-2 px-3 py-2 bg-surface-2 border border-border rounded-lg text-xs text-muted hover:text-[#e2e8f0] hover:border-border-2 transition-colors"
          >
            <Command className="w-3 h-3" />
            <span>Search...</span>
            <kbd className="ml-auto text-[10px] px-1.5 py-0.5 bg-surface-3 rounded">⌘K</kbd>
          </button>

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
          {/* Top bar */}
          <header className="flex items-center justify-between px-5 py-3 border-b border-border bg-surface shrink-0 md:bg-transparent md:border-0">
            <div className="md:hidden font-mono font-semibold text-sm text-amber">PhantomClaw</div>
            <div className="flex items-center gap-2">
              <NotificationCenter />
            </div>
          </header>

          <div className="flex-1 overflow-y-auto p-5 pb-20 md:pb-5">
            {view === 'deck' && <DeckView snapshot={sseSnapshot} onSnapshot={onSnapshot} />}
            {view === 'chat' && <ChatView />}
            {view === 'providers' && <ProvidersView />}
            {view === 'decisions' && <DecisionsView />}
            {view === 'logs' && <LogsView />}
            {view === 'usage' && <UsageView />}
            {view === 'config' && <ConfigView />}
            {view === 'sessions' && <SessionsView />}
            {view === 'risk' && <RiskView />}
            {view === 'cron' && <CronView />}
            {view === 'feed' && <FeedView />}
            {view === 'theme' && <ThemeSettingsView />}
            {view === 'equity' && <EquityView />}
            {view === 'analytics' && <AnalyticsView />}
            {view === 'diagnostics' && <DiagnosticsView />}
          </div>

          {/* Mobile bottom tabs */}
          <nav className="md:hidden fixed bottom-0 left-0 right-0 bg-surface border-t border-border flex justify-around py-2 z-40">
            {MOBILE_TABS.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setView(tab.id)}
                className={cn(
                  'flex flex-col items-center gap-0.5 px-3 py-1 text-[10px] font-medium transition-colors',
                  view === tab.id ? 'text-amber' : 'text-muted'
                )}
              >
                <tab.icon className="w-5 h-5" />
                <span>{tab.label}</span>
              </button>
            ))}
          </nav>
        </main>

        {/* Command Palette */}
        <CommandPalette
          open={paletteOpen}
          onClose={() => setPaletteOpen(false)}
          onSelect={(id) => setView(id as View)}
        />
      </div>
    </ThemeProvider>
  )
}

function ThemeSettingsView() {
  return (
    <div className="space-y-4">
      <h1 className="text-lg font-semibold">Theme</h1>
      <ThemeSwitcher />
    </div>
  )
}

