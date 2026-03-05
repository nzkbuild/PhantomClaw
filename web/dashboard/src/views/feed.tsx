import { useState, useEffect } from 'react'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ArrowUpCircle, ArrowDownCircle, MinusCircle, Zap } from 'lucide-react'
import { useSSE } from '@/hooks/use-data'

type TradeEvent = {
    id: string
    symbol: string
    action: string
    price: number
    lot: number
    pnl?: number
    ts: string
}

export function FeedView() {
    const [events, setEvents] = useState<TradeEvent[]>([])

    useSSE('/api/events', {
        onSnapshot: (data) => {
            const d = data as { trades?: TradeEvent[] }
            if (d.trades) setEvents((prev) => [...d.trades!, ...prev].slice(0, 100))
        },
    })

    // Also poll decisions as feed items
    useEffect(() => {
        fetch('/api/decisions?limit=20')
            .then((r) => r.json())
            .then((d) => {
                const decisions = (d.decisions || []).map((dec: Record<string, unknown>) => ({
                    id: `dec-${dec.id}`,
                    symbol: dec.symbol as string,
                    action: dec.decision as string,
                    price: 0,
                    lot: 0,
                    ts: dec.created_at as string,
                }))
                setEvents((prev) => [...decisions, ...prev].slice(0, 100))
            })
            .catch(() => { })
    }, [])

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Live Feed</h1>
                <div className="flex items-center gap-2">
                    <div className="w-2 h-2 rounded-full bg-ok animate-pulse" />
                    <span className="text-[11px] text-muted font-mono">Streaming</span>
                </div>
            </div>

            <div className="space-y-2">
                {events.length === 0 && (
                    <div className="text-center p-12 text-muted text-sm">
                        <Zap className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                        Waiting for trade events...
                    </div>
                )}
                {events.map((ev) => (
                    <Card key={ev.id} className="bg-surface border-border p-3 animate-in fade-in slide-in-from-top-2 duration-300">
                        <div className="flex items-center gap-3">
                            <ActionIcon action={ev.action} />
                            <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-2">
                                    <span className="font-mono font-semibold text-sm">{ev.symbol}</span>
                                    <ActionBadge action={ev.action} />
                                </div>
                                <div className="flex gap-4 text-[11px] text-muted font-mono mt-0.5">
                                    {ev.price > 0 && <span>@ {ev.price.toFixed(5)}</span>}
                                    {ev.lot > 0 && <span>{ev.lot} lot</span>}
                                    {ev.pnl !== undefined && (
                                        <span className={ev.pnl >= 0 ? 'text-ok' : 'text-bad'}>
                                            {ev.pnl >= 0 ? '+' : ''}{ev.pnl.toFixed(2)}
                                        </span>
                                    )}
                                </div>
                            </div>
                            <span className="font-mono text-[10px] text-muted-2 shrink-0">
                                {new Date(ev.ts).toLocaleTimeString()}
                            </span>
                        </div>
                    </Card>
                ))}
            </div>
        </div>
    )
}

function ActionIcon({ action }: { action: string }) {
    const lower = action.toLowerCase()
    if (lower.includes('buy') || lower.includes('long'))
        return <ArrowUpCircle className="w-5 h-5 text-ok shrink-0" />
    if (lower.includes('sell') || lower.includes('short') || lower.includes('close'))
        return <ArrowDownCircle className="w-5 h-5 text-bad shrink-0" />
    return <MinusCircle className="w-5 h-5 text-muted shrink-0" />
}

function ActionBadge({ action }: { action: string }) {
    const lower = action.toLowerCase()
    const cls = lower.includes('buy') || lower.includes('long')
        ? 'bg-ok/10 text-ok border-ok-dim'
        : lower.includes('sell') || lower.includes('short') || lower.includes('close')
            ? 'bg-bad/10 text-bad border-bad-dim'
            : 'bg-surface-3 text-muted border-border'
    return <Badge variant="outline" className={`font-mono text-[10px] ${cls}`}>{action}</Badge>
}
