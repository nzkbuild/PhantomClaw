import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { CheckCircle2, AlertTriangle, XCircle } from 'lucide-react'
import { getDiagnostics } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type HealthMap = Record<string, string>
type LLMInfo = { current_provider: string; providers: Record<string, string> }
type RiskInfo = { daily_loss: number; open_positions: number; max_positions: number; halted: boolean }

export function DiagnosticsView() {
    const { data, loading } = useApi(() => getDiagnostics(), [])

    if (loading) return <div className="text-muted text-sm animate-pulse">Loading...</div>
    if (!data) return <div className="text-center p-12 text-muted text-sm">Diagnostics unavailable.</div>

    const mode = (data.mode || 'UNKNOWN') as string
    const session = (data.session || 'N/A') as string
    const health = (data.health || {}) as { overall_ok?: boolean; components?: HealthMap; bridge_host?: string; bridge_port?: number }
    const llm = (data.llm || {}) as LLMInfo
    const risk = (data.risk || {}) as RiskInfo

    const components = health.components || {}
    const overallOk = health.overall_ok ?? false

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Diagnostics</h1>
                <Badge variant="outline" className={`font-mono text-xs ${overallOk ? 'text-ok border-ok-dim' : 'text-bad border-bad-dim'}`}>
                    {overallOk ? '✓ HEALTHY' : '⚠ DEGRADED'}
                </Badge>
            </div>

            {/* Overview Cards */}
            <div className="grid grid-cols-3 gap-3">
                <InfoCard label="MODE" value={mode} accent={mode === 'HALT' ? 'text-bad' : 'text-amber'} />
                <InfoCard label="SESSION" value={session} />
                <InfoCard label="PROVIDER" value={llm.current_provider || 'N/A'} />
            </div>

            {/* Component Health Grid */}
            <Card className="bg-surface border-border">
                <CardContent className="p-4">
                    <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-3">Component Health</div>
                    <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
                        {Object.entries(components).map(([name, status]) => (
                            <div key={name} className="flex items-center gap-2 p-2.5 bg-surface-2 rounded-lg border border-border">
                                <StatusIcon status={status} />
                                <div className="min-w-0">
                                    <div className="text-xs font-medium truncate">{name}</div>
                                    <div className={`text-[10px] font-mono ${statusColor(status)}`}>{status}</div>
                                </div>
                            </div>
                        ))}
                        {Object.keys(components).length === 0 && (
                            <div className="col-span-full text-center p-4 text-muted text-sm">No components registered.</div>
                        )}
                    </div>
                </CardContent>
            </Card>

            {/* Provider Status */}
            <Card className="bg-surface border-border">
                <CardContent className="p-4">
                    <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-3">LLM Providers</div>
                    <div className="space-y-2">
                        {Object.entries(llm.providers || {}).map(([name, status]) => (
                            <div key={name} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                <div className="flex items-center gap-2">
                                    <div className={`w-2 h-2 rounded-full ${status === 'healthy' ? 'bg-ok' : status.includes('cooldown') ? 'bg-bad' : 'bg-warn'}`} />
                                    <span className="text-sm font-medium">{name}</span>
                                    {name === llm.current_provider && <Badge variant="outline" className="text-[9px] px-1 py-0 text-amber border-amber-dim">PRIMARY</Badge>}
                                </div>
                                <span className="font-mono text-[11px] text-muted">{status}</span>
                            </div>
                        ))}
                    </div>
                </CardContent>
            </Card>

            {/* Risk Quick View */}
            <Card className="bg-surface border-border">
                <CardContent className="p-4">
                    <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-3">Risk Snapshot</div>
                    <div className="grid grid-cols-2 gap-3 text-sm">
                        <Row label="Daily Loss" value={`$${(risk.daily_loss ?? 0).toFixed(2)}`} warn={(risk.daily_loss ?? 0) > 0} />
                        <Row label="Open Positions" value={`${risk.open_positions ?? 0} / ${risk.max_positions ?? 0}`} />
                        <Row label="Halted" value={risk.halted ? 'YES' : 'NO'} warn={risk.halted} />
                        <Row label="Bridge" value={`${health.bridge_host || '?'}:${health.bridge_port || '?'}`} />
                    </div>
                </CardContent>
            </Card>
        </div>
    )
}

function StatusIcon({ status }: { status: string }) {
    if (status === 'healthy') return <CheckCircle2 className="w-4 h-4 text-ok shrink-0" />
    if (status === 'degraded') return <AlertTriangle className="w-4 h-4 text-warn shrink-0" />
    return <XCircle className="w-4 h-4 text-bad shrink-0" />
}

function statusColor(status: string) {
    if (status === 'healthy') return 'text-ok'
    if (status === 'degraded') return 'text-warn'
    return 'text-bad'
}

function InfoCard({ label, value, accent }: { label: string; value: string; accent?: string }) {
    return (
        <Card className="bg-surface border-border">
            <CardContent className="p-4">
                <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-1">{label}</div>
                <div className={`font-mono text-lg font-semibold truncate ${accent || ''}`}>{value}</div>
            </CardContent>
        </Card>
    )
}

function Row({ label, value, warn }: { label: string; value: string; warn?: boolean }) {
    return (
        <div className="flex justify-between items-center py-1 border-b border-border/50">
            <span className="text-muted text-xs">{label}</span>
            <span className={`font-mono text-xs font-medium ${warn ? 'text-bad' : ''}`}>{value}</span>
        </div>
    )
}
