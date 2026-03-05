import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ShieldAlert, TrendingDown, BarChart3, AlertTriangle } from 'lucide-react'
import { api } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type RiskData = {
    max_lot: number
    max_daily_loss: number
    current_daily_loss: number
    max_drawdown_pct: number
    current_drawdown_pct: number
    open_positions: number
    max_positions: number
    daily_loss_pct: number
}

export function RiskView() {
    const { data, loading } = useApi(
        () => api<{ risk: RiskData }>('/api/risk'),
        []
    )

    const risk = data?.risk

    if (loading) return <div className="text-muted text-sm animate-pulse">Loading...</div>
    if (!risk) return <div className="text-center p-12 text-muted text-sm">Risk data unavailable.</div>

    const lossRatio = risk.max_daily_loss > 0 ? (risk.current_daily_loss / risk.max_daily_loss) * 100 : 0
    const ddRatio = risk.max_drawdown_pct > 0 ? (risk.current_drawdown_pct / risk.max_drawdown_pct) * 100 : 0
    const posRatio = risk.max_positions > 0 ? (risk.open_positions / risk.max_positions) * 100 : 0

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Risk Dashboard</h1>
                {lossRatio > 80 && <Badge variant="outline" className="text-bad border-bad-dim font-mono text-xs animate-pulse">⚠ NEAR LIMIT</Badge>}
            </div>

            {/* Gauges */}
            <div className="grid grid-cols-3 gap-3">
                <GaugeCard icon={TrendingDown} label="DAILY LOSS" value={`$${risk.current_daily_loss.toFixed(2)}`} max={`$${risk.max_daily_loss.toFixed(2)}`} pct={lossRatio} />
                <GaugeCard icon={BarChart3} label="DRAWDOWN" value={`${risk.current_drawdown_pct.toFixed(1)}%`} max={`${risk.max_drawdown_pct}%`} pct={ddRatio} />
                <GaugeCard icon={ShieldAlert} label="POSITIONS" value={`${risk.open_positions}`} max={`${risk.max_positions}`} pct={posRatio} />
            </div>

            {/* Detail Cards */}
            <div className="grid grid-cols-2 gap-3">
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-3">Risk Limits</div>
                        <div className="space-y-2 text-sm">
                            <Row label="Max Lot Size" value={risk.max_lot.toFixed(2)} />
                            <Row label="Max Daily Loss" value={`$${risk.max_daily_loss.toFixed(2)}`} />
                            <Row label="Max Drawdown" value={`${risk.max_drawdown_pct}%`} />
                            <Row label="Max Positions" value={`${risk.max_positions}`} />
                        </div>
                    </CardContent>
                </Card>
                <Card className="bg-surface border-border">
                    <CardContent className="p-4">
                        <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-3">Current Status</div>
                        <div className="space-y-2 text-sm">
                            <Row label="Daily Loss Used" value={`${lossRatio.toFixed(0)}%`} warn={lossRatio > 70} />
                            <Row label="Drawdown Used" value={`${ddRatio.toFixed(0)}%`} warn={ddRatio > 70} />
                            <Row label="Position Slots" value={`${risk.open_positions}/${risk.max_positions}`} warn={posRatio >= 100} />
                        </div>
                    </CardContent>
                </Card>
            </div>
        </div>
    )
}

function GaugeCard({ icon: Icon, label, value, max, pct }: { icon: typeof AlertTriangle; label: string; value: string; max: string; pct: number }) {
    const barColor = pct > 80 ? 'bg-bad' : pct > 50 ? 'bg-warn' : 'bg-ok'
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="flex items-center gap-2 mb-2">
                    <Icon className="w-3.5 h-3.5 text-muted" />
                    <span className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted">{label}</span>
                </div>
                <div className="font-mono text-xl font-semibold mb-1">{value}</div>
                <div className="h-2 bg-surface-3 rounded-full overflow-hidden mb-1">
                    <div className={`h-full ${barColor} rounded-full transition-all`} style={{ width: `${Math.min(pct, 100)}%` }} />
                </div>
                <div className="text-[10px] text-muted font-mono">of {max}</div>
            </CardContent>
        </Card>
    )
}

function Row({ label, value, warn }: { label: string; value: string; warn?: boolean }) {
    return (
        <div className="flex justify-between items-center py-1 border-b border-border/50 last:border-0">
            <span className="text-muted text-xs">{label}</span>
            <span className={`font-mono text-xs font-medium ${warn ? 'text-bad' : ''}`}>{value}</span>
        </div>
    )
}
