import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Clock, Play, ToggleLeft, ToggleRight } from 'lucide-react'
import { api, apiPost } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type CronJob = {
    id: string
    name: string
    schedule: string
    enabled: boolean
    last_run: string
    next_run: string
    status: string
}

export function CronView() {
    const { data, loading, refetch } = useApi(
        () => api<{ jobs: CronJob[] }>('/api/cron'),
        []
    )

    const jobs = data?.jobs ?? []

    const fireJob = async (id: string) => {
        try {
            await apiPost(`/api/cron/${id}/fire`)
            refetch()
        } catch { /* ignore */ }
    }

    const toggleJob = async (id: string) => {
        try {
            await apiPost(`/api/cron/${id}/toggle`)
            refetch()
        } catch { /* ignore */ }
    }

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Cron Jobs</h1>
                <Badge variant="outline" className="font-mono text-xs">{jobs.length} jobs</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            <div className="space-y-2">
                {jobs.map((job) => (
                    <Card key={job.id} className="bg-surface border-border">
                        <CardContent className="p-4">
                            <div className="flex items-center justify-between mb-2">
                                <div className="flex items-center gap-2">
                                    <Clock className="w-4 h-4 text-amber" />
                                    <span className="font-semibold text-sm">{job.name}</span>
                                    <Badge
                                        variant="outline"
                                        className={`font-mono text-[10px] ${job.enabled ? 'text-ok border-ok-dim' : 'text-muted border-border'}`}
                                    >
                                        {job.enabled ? 'ACTIVE' : 'DISABLED'}
                                    </Badge>
                                </div>
                                <div className="flex gap-2">
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => toggleJob(job.id)}
                                        className="text-xs border-border hover:border-amber hover:text-amber"
                                    >
                                        {job.enabled ? <ToggleRight className="w-3 h-3 mr-1" /> : <ToggleLeft className="w-3 h-3 mr-1" />}
                                        {job.enabled ? 'Disable' : 'Enable'}
                                    </Button>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => fireJob(job.id)}
                                        className="text-xs border-border hover:border-ok hover:text-ok"
                                    >
                                        <Play className="w-3 h-3 mr-1" /> Run Now
                                    </Button>
                                </div>
                            </div>
                            <div className="flex gap-6 text-[11px] text-muted font-mono">
                                <span>Schedule: {job.schedule}</span>
                                <span>Last: {job.last_run ? new Date(job.last_run).toLocaleString() : 'never'}</span>
                                <span>Next: {job.next_run ? new Date(job.next_run).toLocaleString() : 'N/A'}</span>
                            </div>
                        </CardContent>
                    </Card>
                ))}
                {jobs.length === 0 && !loading && (
                    <div className="text-center p-12 text-muted text-sm">
                        <Clock className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                        No cron jobs configured.
                    </div>
                )}
            </div>
        </div>
    )
}
