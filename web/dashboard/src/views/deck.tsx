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
        getSnapshot().then((r) => {
            setSnap(r.snapshot)
            onSnapshot(r.snapshot)
        }).catch(() => { })
    }

    useEffect(() => { fetchSnap() }, [])
    useInterval(fetchSnap, 5000)
    useSSE('/api/events', {
        onSnapshot: (data) => {
            const d = data as { snapshot?: Snapshot }
            if (d.snapshot) {
                setSnap(d.snapshot)
                onSnapshot(d.snapshot)
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
                    label="DAILY P&L"
                    value={`$${(snap.daily_pnl ?? 0).toFixed(2)}`}
                    accent={(snap.daily_pnl ?? 0) >= 0 ? 'text-ok' : 'text-bad'}
                />
                <KPICard
                    icon={AlertTriangle}
                    label="DAILY LOSS"
                    value={`$${(snap.daily_loss ?? 0).toFixed(2)}`}
                    accent={(snap.daily_loss ?? 0) > 50 ? 'text-bad' : 'text-muted'}
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
                        <div className="font-mono text-xl font-semibold">{((snap.win_rate_7d ?? 0) * 100).toFixed(0)}%</div>
                    </CardContent>
                </Card>
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Provider</div>
                        <div className="flex items-center gap-2">
                            <Badge variant="outline" className="font-mono text-xs">{snap.provider}</Badge>
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
