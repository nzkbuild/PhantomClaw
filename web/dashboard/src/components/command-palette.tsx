import { useState, useEffect, useCallback } from 'react'
import { cn } from '@/lib/utils'
import {
    LayoutDashboard, MessageSquare, Cpu, Gauge, ShieldAlert,
    Settings, ScrollText, Clock, History, TrendingUp,
    BarChart3, Activity, FolderOpen, Zap, Search, X,
} from 'lucide-react'

type PaletteItem = {
    id: string
    label: string
    icon: typeof LayoutDashboard
    group: string
}

const COMMANDS: PaletteItem[] = [
    { id: 'deck', label: 'Control Deck', icon: LayoutDashboard, group: 'Navigate' },
    { id: 'chat', label: 'Chat', icon: MessageSquare, group: 'Navigate' },
    { id: 'equity', label: 'Equity', icon: TrendingUp, group: 'Navigate' },
    { id: 'decisions', label: 'Decisions', icon: History, group: 'Navigate' },
    { id: 'analytics', label: 'Analytics', icon: BarChart3, group: 'Navigate' },
    { id: 'feed', label: 'Live Feed', icon: Zap, group: 'Navigate' },
    { id: 'usage', label: 'Usage', icon: Gauge, group: 'Data' },
    { id: 'sessions', label: 'Sessions', icon: FolderOpen, group: 'Data' },
    { id: 'risk', label: 'Risk Dashboard', icon: ShieldAlert, group: 'Data' },
    { id: 'cron', label: 'Cron Jobs', icon: Clock, group: 'Data' },
    { id: 'providers', label: 'Providers', icon: Cpu, group: 'System' },
    { id: 'diagnostics', label: 'Diagnostics', icon: Activity, group: 'System' },
    { id: 'logs', label: 'Logs', icon: ScrollText, group: 'System' },
    { id: 'config', label: 'Config', icon: Settings, group: 'System' },
]

type Props = {
    open: boolean
    onClose: () => void
    onSelect: (id: string) => void
}

export function CommandPalette({ open, onClose, onSelect }: Props) {
    const [query, setQuery] = useState('')
    const [selected, setSelected] = useState(0)

    const filtered = COMMANDS.filter((c) =>
        c.label.toLowerCase().includes(query.toLowerCase())
    )

    const handleSelect = useCallback((id: string) => {
        onSelect(id)
        onClose()
        setQuery('')
        setSelected(0)
    }, [onSelect, onClose])

    useEffect(() => {
        if (!open) return
        setQuery('')
        setSelected(0)
    }, [open])

    useEffect(() => {
        if (!open) return
        const handler = (e: KeyboardEvent) => {
            if (e.key === 'Escape') { onClose(); return }
            if (e.key === 'ArrowDown') {
                e.preventDefault()
                setSelected((s) => Math.min(s + 1, filtered.length - 1))
            }
            if (e.key === 'ArrowUp') {
                e.preventDefault()
                setSelected((s) => Math.max(s - 1, 0))
            }
            if (e.key === 'Enter' && filtered[selected]) {
                handleSelect(filtered[selected].id)
            }
        }
        window.addEventListener('keydown', handler)
        return () => window.removeEventListener('keydown', handler)
    }, [open, filtered, selected, handleSelect, onClose])

    if (!open) return null

    return (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]" onClick={onClose}>
            {/* Backdrop */}
            <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />

            {/* Palette */}
            <div
                className="relative w-[520px] bg-surface border border-border-2 rounded-xl shadow-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-150"
                onClick={(e) => e.stopPropagation()}
            >
                {/* Input */}
                <div className="flex items-center gap-3 px-4 py-3 border-b border-border">
                    <Search className="w-4 h-4 text-muted shrink-0" />
                    <input
                        autoFocus
                        value={query}
                        onChange={(e) => { setQuery(e.target.value); setSelected(0) }}
                        placeholder="Type a command..."
                        className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-2"
                    />
                    <button onClick={onClose} className="text-muted hover:text-[#e2e8f0]">
                        <X className="w-4 h-4" />
                    </button>
                </div>

                {/* Results */}
                <div className="max-h-[300px] overflow-y-auto py-1">
                    {filtered.length === 0 && (
                        <div className="px-4 py-6 text-center text-sm text-muted">No results</div>
                    )}
                    {(() => {
                        let lastGroup = ''
                        return filtered.map((cmd, i) => {
                            const showGroup = cmd.group !== lastGroup
                            lastGroup = cmd.group
                            return (
                                <div key={cmd.id}>
                                    {showGroup && (
                                        <div className="px-4 py-1.5 text-[10px] font-semibold tracking-[.12em] uppercase text-muted-2">
                                            {cmd.group}
                                        </div>
                                    )}
                                    <button
                                        onClick={() => handleSelect(cmd.id)}
                                        onMouseEnter={() => setSelected(i)}
                                        className={cn(
                                            'w-full flex items-center gap-3 px-4 py-2 text-sm transition-colors',
                                            i === selected ? 'bg-amber/10 text-amber' : 'text-[#e2e8f0] hover:bg-surface-2'
                                        )}
                                    >
                                        <cmd.icon className="w-4 h-4 shrink-0 opacity-60" />
                                        <span>{cmd.label}</span>
                                        <span className="ml-auto text-[10px] text-muted-2 font-mono">{cmd.group.toLowerCase()}</span>
                                    </button>
                                </div>
                            )
                        })
                    })()}
                </div>

                {/* Footer */}
                <div className="flex items-center gap-3 px-4 py-2 border-t border-border text-[10px] text-muted-2">
                    <span><kbd className="px-1 py-0.5 bg-surface-3 rounded text-[9px]">↑↓</kbd> navigate</span>
                    <span><kbd className="px-1 py-0.5 bg-surface-3 rounded text-[9px]">↵</kbd> select</span>
                    <span><kbd className="px-1 py-0.5 bg-surface-3 rounded text-[9px]">esc</kbd> close</span>
                </div>
            </div>
        </div>
    )
}
