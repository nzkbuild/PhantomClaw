import { useEffect, useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Shield, Clock, TrendingUp, AlertTriangle } from 'lucide-react'
import { getSnapshot, switchMode, type Snapshot } from '@/lib/api'
import { useSSE, useInterval } from '@/hooks/use-data'

const MODE_COLORS: Record<string, string> = {
    OBSERVE: 'bg-cyan/10 text-cyan border-cyan-dim',
    SUGGEST: 'bg-amber/10 text-amber border-amber-dim',
    AUTO: 'bg-ok/10 text-ok border-ok-dim',
    HALT: 'bg-bad/10 text-bad border-bad-dim',
}

const MODES = ['OBSERVE', 'SUGGEST', 'AUTO', 'HALT'] as const

export function DeckView({ snapshot: _sseSnapshot, onSnapshot }: { snapshot: unknown; onSnapshot: (d: unknown) => void }) {
    const [snap, setSnap] = useState<Snapshot | null>(null)
    const [switching, setSwitching] = useState(false)

    const fetchSnap = () => {
        getSnapshot().then((snap) => {
            setSnap(snap)
            onSnapshot(snap)
        }).catch(() => { })
    }

    useEffect(() => { fetchSnap() }, [])
    useInterval(fetchSnap, 5000)
    useSSE('/api/events', {
        onSnapshot: (data) => {
            const d = data as { snapshot?: Snapshot } & Snapshot
            // SSE may wrap in snapshot key or send flat
            const s = d.snapshot || d
            if (s.mode) {
                setSnap(s)
                onSnapshot(s)
            }
        },
    })

    const handleMode = async (mode: string) => {
        setSwitching(true)
        try {
            await switchMode(mode)
            fetchSnap()
        } catch { /* ignore */ }
        finally { setSwitching(false) }
    }

    if (!snap) return <div className="text-muted text-sm animate-pulse">Loading...</div>

    return (
        <div className="space-y-4">
            {/* ── Mode Switcher ── */}
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Control Deck</h1>
                <div className="flex gap-2">
                    {MODES.map((m) => (
                        <Button
                            key={m}
                            variant={snap.mode === m ? 'default' : 'outline'}
                            size="sm"
                            disabled={switching || snap.mode === m}
                            onClick={() => handleMode(m)}
                            className={snap.mode === m ? MODE_COLORS[m] : 'border-border text-muted hover:text-[#e2e8f0]'}
                        >
                            {m === 'HALT' && <AlertTriangle className="w-3.5 h-3.5 mr-1" />}
                            {m}
                        </Button>
                    ))}
                </div>
            </div>

            {/* ── KPI Cards ── */}
            <div className="grid grid-cols-4 gap-3">
                <KPICard icon={Shield} label="MODE" value={snap.mode} accent={MODE_COLORS[snap.mode] || ''} />
                <KPICard icon={Clock} label="SESSION" value={snap.session || 'N/A'} />
                <KPICard
                    icon={TrendingUp}
                    label="DAILY LOSS"
                    value={`$${(snap.daily_loss ?? 0).toFixed(2)}`}
                    accent={(snap.daily_loss ?? 0) > 0 ? 'text-bad' : 'text-ok'}
                />
                <KPICard
                    icon={AlertTriangle}
                    label="MAX LOSS"
                    value={`$${(snap.max_daily_loss ?? 0).toFixed(2)}`}
                    accent={(snap.daily_loss ?? 0) > (snap.max_daily_loss ?? 100) * 0.8 ? 'text-bad' : 'text-muted'}
                />
            </div>

            {/* ── Status Row ── */}
            <div className="grid grid-cols-3 gap-3">
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Positions</div>
                        <div className="font-mono text-xl font-semibold">
                            {snap.open_positions}/{snap.max_positions}
                        </div>
                    </CardContent>
                </Card>
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Win Rate (7d)</div>
                        <div className="font-mono text-xl font-semibold">{snap.session || 'N/A'}</div>
                    </CardContent>
                </Card>
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Status</div>
                        <div className="flex items-center gap-2">
                            <Badge variant="outline" className={`font-mono text-xs ${snap.halted ? 'text-bad border-bad-dim' : 'text-ok border-ok-dim'}`}>
                                {snap.halted ? 'HALTED' : 'RUNNING'}
                            </Badge>
                        </div>
                    </CardContent>
                </Card>
            </div>
        </div>
    )
}

function KPICard({ icon: Icon, label, value, accent }: { icon: typeof Shield; label: string; value: string; accent?: string }) {
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="flex items-center gap-2 mb-2">
                    <Icon className="w-3.5 h-3.5 text-muted" />
                    <span className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted">{label}</span>
                </div>
                <div className={`font-mono text-2xl font-semibold ${accent || ''}`}>{value}</div>
            </CardContent>
        </Card>
    )
}
