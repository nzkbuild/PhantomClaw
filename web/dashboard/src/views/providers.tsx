import { useEffect, useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Star, RefreshCw, Cpu } from 'lucide-react'
import { getDiagnostics, switchModel } from '@/lib/api'

type ProviderEntry = { name: string; status: string; isPrimary: boolean }

export function ProvidersView() {
    const [providers, setProviders] = useState<ProviderEntry[]>([])
    const [current, setCurrent] = useState('')
    const [switching, setSwitching] = useState(false)

    const fetchProviders = () => {
        getDiagnostics().then((diag) => {
            const llm = (diag.llm || {}) as Record<string, unknown>
            const providerStatus = (llm.providers || {}) as Record<string, string>
            const primary = (llm.current_provider || '') as string
            setCurrent(primary)
            const entries = Object.entries(providerStatus).map(([name, status]) => ({
                name,
                status: status as string,
                isPrimary: name === primary,
            }))
            setProviders(entries)
        }).catch(() => { })
    }

    useEffect(() => { fetchProviders() }, [])

    const handleSwitch = async (name: string) => {
        setSwitching(true)
        try {
            await switchModel(name)
            fetchProviders()
        } catch { /* ignore */ }
        finally { setSwitching(false) }
    }

    const statusColor = (s: string) => {
        if (s === 'healthy') return 'bg-ok'
        if (s.includes('cooldown')) return 'bg-bad'
        if (s.includes('degraded')) return 'bg-warn'
        return 'bg-muted'
    }

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Providers</h1>
                <Badge variant="outline" className="font-mono text-xs">Primary: {current}</Badge>
            </div>

            <div className="space-y-2">
                {providers.map((p) => (
                    <Card key={p.name} className="bg-surface border-border hover:border-border-2 transition-colors">
                        <CardContent className="p-4 flex items-center gap-4">
                            <div className={`w-2.5 h-2.5 rounded-full ${statusColor(p.status)}`} />
                            <div className="flex-1">
                                <div className="flex items-center gap-2">
                                    <span className="font-semibold text-sm">{p.name}</span>
                                    {p.isPrimary && <Star className="w-3.5 h-3.5 text-amber fill-amber" />}
                                </div>
                                <span className="font-mono text-[11px] text-muted">{p.status}</span>
                            </div>
                            <div className="flex gap-2">
                                {!p.isPrimary && (
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={switching}
                                        onClick={() => handleSwitch(p.name)}
                                        className="text-xs border-border hover:border-amber hover:text-amber"
                                    >
                                        <Cpu className="w-3 h-3 mr-1" />
                                        Make Primary
                                    </Button>
                                )}
                                {p.status.includes('cooldown') && (
                                    <Button variant="outline" size="sm" className="text-xs border-border hover:border-ok hover:text-ok">
                                        <RefreshCw className="w-3 h-3 mr-1" />
                                        Reset
                                    </Button>
                                )}
                            </div>
                        </CardContent>
                    </Card>
                ))}
            </div>
        </div>
    )
}
