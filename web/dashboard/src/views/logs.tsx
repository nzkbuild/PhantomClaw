import { useEffect, useState, useRef } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ScrollText } from 'lucide-react'
import { getLogs } from '@/lib/api'
import { useSSE } from '@/hooks/use-data'

type LogEntry = {
    ts: string
    level: string
    msg: string
    caller?: string
}

const LEVEL_COLORS: Record<string, string> = {
    error: 'bg-bad/10 text-bad',
    warn: 'bg-warn/10 text-warn',
    info: 'bg-cyan/5 text-cyan',
    debug: 'bg-surface-3 text-muted',
}

export function LogsView() {
    const [logs, setLogs] = useState<LogEntry[]>([])
    const [autoScroll, setAutoScroll] = useState(true)
    const bottomRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
        getLogs(100).then((r) => {
            const entries = (r.logs || []) as LogEntry[]
            setLogs(entries)
        }).catch(() => { })
    }, [])

    useSSE('/api/events', {
        onLog: (data) => {
            const entry = data as { logs?: LogEntry[] }
            if (entry.logs) {
                setLogs((prev) => [...prev.slice(-500), ...entry.logs!])
            }
        },
    })

    useEffect(() => {
        if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }, [logs, autoScroll])

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Logs</h1>
                <div className="flex items-center gap-3">
                    <label className="flex items-center gap-2 text-sm text-muted cursor-pointer">
                        <input
                            type="checkbox"
                            checked={autoScroll}
                            onChange={(e) => setAutoScroll(e.target.checked)}
                            className="accent-amber"
                        />
                        Auto-scroll
                    </label>
                    <Badge variant="outline" className="font-mono text-xs">{logs.length} entries</Badge>
                </div>
            </div>

            <Card className="bg-surface border-border">
                <CardContent className="p-0">
                    <div className="max-h-[calc(100vh-180px)] overflow-y-auto">
                        {logs.length === 0 && (
                            <div className="text-center p-12 text-muted text-sm">
                                <ScrollText className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                                No logs yet.
                            </div>
                        )}
                        {logs.map((log, i) => (
                            <div key={i} className="flex gap-2 px-3 py-1.5 border-b border-border/50 last:border-0 hover:bg-surface-2 transition-colors text-[11px] font-mono">
                                <span className="text-muted-2 shrink-0 w-[145px]">{log.ts}</span>
                                <span className={`shrink-0 w-[42px] text-center rounded px-1 py-0.5 text-[10px] ${LEVEL_COLORS[log.level] || LEVEL_COLORS.debug}`}>
                                    {log.level}
                                </span>
                                <span className="text-[#e2e8f0] truncate flex-1">{log.msg}</span>
                                {log.caller && <span className="text-muted-2 shrink-0">{log.caller}</span>}
                            </div>
                        ))}
                        <div ref={bottomRef} />
                    </div>
                </CardContent>
            </Card>
        </div>
    )
}
