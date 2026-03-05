import { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Zap, Clock, Hash, DollarSign } from 'lucide-react'
import { useApi } from '@/hooks/use-data'
import { api } from '@/lib/api'

type ProviderUsage = {
    name: string
    requests: number
    tokens: number
    avg_latency_ms: number
    errors: number
}

export function UsageView() {
    const [days] = useState(7)
    const { data, loading } = useApi(
        () => api<{ usage: ProviderUsage[]; total_tokens: number; total_requests: number }>(`/api/usage?days=${days}`),
        [days]
    )

    const usage = data?.usage ?? []
    const totalTokens = data?.total_tokens ?? 0
    const totalRequests = data?.total_requests ?? 0

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Usage</h1>
                <Badge variant="outline" className="font-mono text-xs">Last {days} days</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            {/* Summary Cards */}
            <div className="grid grid-cols-3 gap-3">
                <SummaryCard icon={Hash} label="TOTAL REQUESTS" value={totalRequests.toLocaleString()} />
                <SummaryCard icon={Zap} label="TOTAL TOKENS" value={totalTokens.toLocaleString()} />
                <SummaryCard icon={DollarSign} label="EST. COST" value={`$${(totalTokens * 0.000003).toFixed(4)}`} />
            </div>

            {/* Per-Provider */}
            <div className="space-y-2">
                {usage.map((p) => (
                    <Card key={p.name} className="bg-surface border-border">
                        <CardContent className="p-4">
                            <div className="flex items-center justify-between mb-3">
                                <span className="font-semibold text-sm">{p.name}</span>
                                <Badge variant="outline" className="font-mono text-[11px]">{p.requests} calls</Badge>
                            </div>
                            {/* Token bar */}
                            <div className="mb-2">
                                <div className="flex justify-between text-[10px] text-muted mb-1">
                                    <span>Tokens</span>
                                    <span className="font-mono">{p.tokens.toLocaleString()}</span>
                                </div>
                                <div className="h-2 bg-surface-3 rounded-full overflow-hidden">
                                    <div
                                        className="h-full bg-amber rounded-full transition-all"
                                        style={{ width: `${totalTokens > 0 ? (p.tokens / totalTokens) * 100 : 0}%` }}
                                    />
                                </div>
                            </div>
                            <div className="flex gap-6 text-[11px] text-muted font-mono">
                                <span className="flex items-center gap-1"><Clock className="w-3 h-3" /> {p.avg_latency_ms}ms avg</span>
                                {p.errors > 0 && <span className="text-bad">{p.errors} errors</span>}
                            </div>
                        </CardContent>
                    </Card>
                ))}
                {usage.length === 0 && !loading && (
                    <div className="text-center p-8 text-muted text-sm">No usage data recorded yet.</div>
                )}
            </div>
        </div>
    )
}

function SummaryCard({ icon: Icon, label, value }: { icon: typeof Zap; label: string; value: string }) {
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="flex items-center gap-2 mb-2">
                    <Icon className="w-3.5 h-3.5 text-muted" />
                    <span className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted">{label}</span>
                </div>
                <div className="font-mono text-xl font-semibold">{value}</div>
            </CardContent>
        </Card>
    )
}
