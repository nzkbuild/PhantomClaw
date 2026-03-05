import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { TrendingUp } from 'lucide-react'
import { api } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type EquityPoint = {
    date: string
    equity: number
    balance: number
}

export function EquityView() {
    const { data, loading } = useApi(
        () => api<{ days: number; points: EquityPoint[] }>('/api/equity?days=30'),
        []
    )

    const points = data?.points ?? []
    const hasData = points.length > 0

    // Compute stats
    const first = points[0]?.equity ?? 0
    const last = points[points.length - 1]?.equity ?? 0
    const change = last - first
    const changePct = first > 0 ? ((change / first) * 100).toFixed(2) : '0.00'
    const max = Math.max(...points.map((p) => p.equity), 0)

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Equity Curve</h1>
                <Badge variant="outline" className="font-mono text-xs">Last {data?.days ?? 30} days</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            {/* KPI Cards */}
            {hasData && (
                <div className="grid grid-cols-4 gap-3">
                    <StatCard label="CURRENT" value={`$${last.toFixed(2)}`} />
                    <StatCard label="CHANGE" value={`${change >= 0 ? '+' : ''}$${change.toFixed(2)}`} accent={change >= 0 ? 'text-ok' : 'text-bad'} />
                    <StatCard label="CHANGE %" value={`${Number(changePct) >= 0 ? '+' : ''}${changePct}%`} accent={Number(changePct) >= 0 ? 'text-ok' : 'text-bad'} />
                    <StatCard label="HIGH" value={`$${max.toFixed(2)}`} />
                </div>
            )}

            {/* SVG Chart */}
            <Card className="bg-surface border-border">
                <CardContent className="p-4">
                    {hasData ? (
                        <EquityChart points={points} />
                    ) : !loading ? (
                        <div className="flex flex-col items-center justify-center h-48 text-muted text-sm">
                            <TrendingUp className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                            No equity data recorded yet.
                        </div>
                    ) : null}
                </CardContent>
            </Card>
        </div>
    )
}

function EquityChart({ points }: { points: EquityPoint[] }) {
    const W = 800, H = 250, PAD = 40
    const values = points.map((p) => p.equity)
    const max = Math.max(...values)
    const min = Math.min(...values)
    const range = max - min || 1

    const toX = (i: number) => PAD + (i / (points.length - 1)) * (W - PAD * 2)
    const toY = (v: number) => PAD + (1 - (v - min) / range) * (H - PAD * 2)

    const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${toX(i).toFixed(1)},${toY(p.equity).toFixed(1)}`).join(' ')
    const areaPath = linePath + ` L${toX(points.length - 1).toFixed(1)},${H - PAD} L${PAD},${H - PAD} Z`

    const isUp = (points[points.length - 1]?.equity ?? 0) >= (points[0]?.equity ?? 0)
    const color = isUp ? '#22c55e' : '#ef4444'

    // Y-axis labels
    const ySteps = 5
    const yLabels = Array.from({ length: ySteps + 1 }, (_, i) => min + (range / ySteps) * i)

    return (
        <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto" preserveAspectRatio="xMidYMid meet">
            {/* Grid lines */}
            {yLabels.map((v, i) => (
                <g key={i}>
                    <line x1={PAD} y1={toY(v)} x2={W - PAD} y2={toY(v)} stroke="#1e2d3d" strokeWidth={0.5} />
                    <text x={PAD - 4} y={toY(v) + 3} textAnchor="end" fill="#64748b" fontSize={9} fontFamily="JetBrains Mono">
                        {v.toFixed(0)}
                    </text>
                </g>
            ))}
            {/* X-axis labels (every 5th) */}
            {points.filter((_, i) => i % Math.max(1, Math.floor(points.length / 6)) === 0).map((p, i) => (
                <text key={i} x={toX(points.indexOf(p))} y={H - PAD + 14} textAnchor="middle" fill="#64748b" fontSize={8} fontFamily="JetBrains Mono">
                    {new Date(p.date).toLocaleDateString('en', { month: 'short', day: 'numeric' })}
                </text>
            ))}
            {/* Area fill */}
            <path d={areaPath} fill={color} opacity={0.08} />
            {/* Line */}
            <path d={linePath} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
            {/* Dot on last point */}
            <circle cx={toX(points.length - 1)} cy={toY(values[values.length - 1])} r={3.5} fill={color} />
        </svg>
    )
}

function StatCard({ label, value, accent }: { label: string; value: string; accent?: string }) {
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-1">{label}</div>
                <div className={`font-mono text-lg font-semibold ${accent || ''}`}>{value}</div>
            </CardContent>
        </Card>
    )
}
