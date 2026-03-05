import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { BarChart3, TrendingUp, Target, Percent } from 'lucide-react'
import { api } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type PairStat = {
    symbol: string
    trades: number
    wins: number
    pnl: number
    avg_pnl: number
}

type AnalyticsSummary = {
    total_trades: number
    win_rate: number
    total_pnl: number
    avg_pnl: number
    best_pair: string
    worst_pair: string
}

export function AnalyticsView() {
    const { data, loading } = useApi(
        () => api<{ days: number; summary: AnalyticsSummary; pairs: PairStat[] }>('/api/analytics?days=7'),
        []
    )

    const summary = data?.summary
    const pairs = data?.pairs ?? []

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Analytics</h1>
                <Badge variant="outline" className="font-mono text-xs">Last {data?.days ?? 7} days</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            {/* Summary Cards */}
            {summary && (
                <div className="grid grid-cols-4 gap-3">
                    <KPI icon={Target} label="TRADES" value={`${summary.total_trades}`} />
                    <KPI icon={Percent} label="WIN RATE" value={`${(summary.win_rate * 100).toFixed(1)}%`} accent={summary.win_rate >= 0.5 ? 'text-ok' : 'text-bad'} />
                    <KPI icon={TrendingUp} label="TOTAL P&L" value={`$${summary.total_pnl.toFixed(2)}`} accent={summary.total_pnl >= 0 ? 'text-ok' : 'text-bad'} />
                    <KPI icon={BarChart3} label="AVG P&L" value={`$${summary.avg_pnl.toFixed(2)}`} accent={summary.avg_pnl >= 0 ? 'text-ok' : 'text-bad'} />
                </div>
            )}

            {/* Per-Pair Table */}
            <Card className="bg-surface border-border overflow-hidden">
                <CardContent className="p-0">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-border">
                                <th className="text-left p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Pair</th>
                                <th className="text-right p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Trades</th>
                                <th className="text-right p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Wins</th>
                                <th className="text-right p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Win %</th>
                                <th className="text-right p-3 text-[11px] font-medium tracking-wide uppercase text-muted">P&L</th>
                                <th className="text-right p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Avg P&L</th>
                            </tr>
                        </thead>
                        <tbody>
                            {pairs.length === 0 && !loading && (
                                <tr><td colSpan={6} className="text-center p-8 text-muted text-sm">
                                    <BarChart3 className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                                    No trade data yet.
                                </td></tr>
                            )}
                            {pairs.map((p) => {
                                const winPct = p.trades > 0 ? ((p.wins / p.trades) * 100).toFixed(0) : '0'
                                return (
                                    <tr key={p.symbol} className="border-b border-border last:border-0 hover:bg-surface-2 transition-colors">
                                        <td className="p-3 font-mono font-medium">{p.symbol}</td>
                                        <td className="p-3 text-right font-mono">{p.trades}</td>
                                        <td className="p-3 text-right font-mono">{p.wins}</td>
                                        <td className="p-3 text-right font-mono">
                                            <span className={Number(winPct) >= 50 ? 'text-ok' : 'text-bad'}>{winPct}%</span>
                                        </td>
                                        <td className="p-3 text-right font-mono">
                                            <span className={p.pnl >= 0 ? 'text-ok' : 'text-bad'}>
                                                {p.pnl >= 0 ? '+' : ''}{p.pnl.toFixed(2)}
                                            </span>
                                        </td>
                                        <td className="p-3 text-right font-mono text-muted">{p.avg_pnl.toFixed(2)}</td>
                                    </tr>
                                )
                            })}
                        </tbody>
                    </table>
                </CardContent>
            </Card>
        </div>
    )
}

function KPI({ icon: Icon, label, value, accent }: { icon: typeof Target; label: string; value: string; accent?: string }) {
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="flex items-center gap-2 mb-2">
                    <Icon className="w-3.5 h-3.5 text-muted" />
                    <span className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted">{label}</span>
                </div>
                <div className={`font-mono text-xl font-semibold ${accent || ''}`}>{value}</div>
            </CardContent>
        </Card>
    )
}
