import { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { History } from 'lucide-react'
import { getDecisions, type Decision } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

export function DecisionsView() {
    const [limit] = useState(20)
    const [symbol] = useState('')
    const { data, loading } = useApi(() => getDecisions(limit, symbol), [limit, symbol])

    const decisions = (data?.decisions ?? []) as Decision[]

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Decisions</h1>
                <Badge variant="outline" className="font-mono text-xs">{decisions.length} records</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            <Card className="bg-surface border-border overflow-hidden">
                <CardContent className="p-0">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-border">
                                <th className="text-left p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Symbol</th>
                                <th className="text-left p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Decision</th>
                                <th className="text-left p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Reason</th>
                                <th className="text-left p-3 text-[11px] font-medium tracking-wide uppercase text-muted">Time</th>
                            </tr>
                        </thead>
                        <tbody>
                            {decisions.length === 0 && (
                                <tr><td colSpan={4} className="text-center p-8 text-muted text-sm">
                                    <History className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                                    No decisions recorded yet.
                                </td></tr>
                            )}
                            {decisions.map((d) => (
                                <tr key={d.id} className="border-b border-border last:border-0 hover:bg-surface-2 transition-colors">
                                    <td className="p-3 font-mono font-medium">{d.symbol}</td>
                                    <td className="p-3">
                                        <DecisionBadge decision={d.decision} />
                                    </td>
                                    <td className="p-3 text-muted max-w-[300px] truncate">{d.reason}</td>
                                    <td className="p-3 font-mono text-[11px] text-muted-2 whitespace-nowrap">
                                        {new Date(d.created_at).toLocaleString()}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </CardContent>
            </Card>
        </div>
    )
}

function DecisionBadge({ decision }: { decision: string }) {
    const lower = decision.toLowerCase()
    const cls = lower.includes('buy') || lower.includes('long')
        ? 'bg-ok/10 text-ok border-ok-dim'
        : lower.includes('sell') || lower.includes('short')
            ? 'bg-bad/10 text-bad border-bad-dim'
            : lower.includes('hold') || lower.includes('wait')
                ? 'bg-amber/10 text-amber border-amber-dim'
                : 'bg-surface-3 text-muted border-border'
    return <Badge variant="outline" className={`font-mono text-[11px] ${cls}`}>{decision}</Badge>
}
