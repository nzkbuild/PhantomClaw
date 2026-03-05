import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { MessageSquare, User, Bot } from 'lucide-react'
import { api } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type SessionEntry = {
    id: string
    turns: number
    last_active: string
    preview: string
}

export function SessionsView() {
    const { data, loading } = useApi(
        () => api<{ sessions: SessionEntry[] }>('/api/sessions/list'),
        []
    )

    const sessions = data?.sessions ?? []

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Sessions</h1>
                <Badge variant="outline" className="font-mono text-xs">{sessions.length} sessions</Badge>
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            <div className="space-y-2">
                {sessions.map((s) => (
                    <Card key={s.id} className="bg-surface border-border hover:border-border-2 transition-colors cursor-pointer">
                        <CardContent className="p-4">
                            <div className="flex items-center justify-between mb-2">
                                <div className="flex items-center gap-2">
                                    <MessageSquare className="w-4 h-4 text-amber" />
                                    <span className="font-mono text-xs text-muted-2">{s.id}</span>
                                </div>
                                <span className="font-mono text-[11px] text-muted">
                                    {new Date(s.last_active).toLocaleString()}
                                </span>
                            </div>
                            <p className="text-sm text-muted truncate">{s.preview || 'No messages'}</p>
                            <div className="flex gap-3 mt-2 text-[11px] text-muted-2 font-mono">
                                <span className="flex items-center gap-1"><User className="w-3 h-3" /><Bot className="w-3 h-3" /> {s.turns} turns</span>
                            </div>
                        </CardContent>
                    </Card>
                ))}
                {sessions.length === 0 && !loading && (
                    <div className="text-center p-12 text-muted text-sm">
                        <MessageSquare className="w-8 h-8 mx-auto mb-2 text-muted-2" />
                        No chat sessions recorded yet.
                    </div>
                )}
            </div>
        </div>
    )
}
