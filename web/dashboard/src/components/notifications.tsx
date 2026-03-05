import { useState, useCallback, useRef, useEffect } from 'react'
import { Bell, X, AlertTriangle, Info, CheckCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSSE } from '@/hooks/use-data'

type Notification = {
    id: string
    type: 'info' | 'warning' | 'success' | 'error'
    title: string
    message: string
    ts: Date
    read: boolean
}

const ICONS = {
    info: Info,
    warning: AlertTriangle,
    success: CheckCircle,
    error: AlertTriangle,
}
const COLORS = {
    info: 'text-cyan',
    warning: 'text-warn',
    success: 'text-ok',
    error: 'text-bad',
}

export function NotificationCenter() {
    const [notifications, setNotifications] = useState<Notification[]>([])
    const [open, setOpen] = useState(false)
    const ref = useRef<HTMLDivElement>(null)

    // Click-away to close
    useEffect(() => {
        if (!open) return
        const handler = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
        }
        document.addEventListener('mousedown', handler)
        return () => document.removeEventListener('mousedown', handler)
    }, [open])

    const addNotification = useCallback((n: Omit<Notification, 'id' | 'ts' | 'read'>) => {
        setNotifications((prev) => [{
            ...n,
            id: `${Date.now()}-${Math.random()}`,
            ts: new Date(),
            read: false,
        }, ...prev].slice(0, 50))
    }, [])

    useSSE('/api/events', {
        onNotification: (data) => {
            const d = data as { type?: string; title?: string; message?: string }
            if (d.title) {
                addNotification({
                    type: (d.type as Notification['type']) || 'info',
                    title: d.title,
                    message: d.message || '',
                })
            }
        },
    })

    const unread = notifications.filter((n) => !n.read).length
    const markAllRead = () => setNotifications((prev) => prev.map((n) => ({ ...n, read: true })))

    return (
        <div className="relative" ref={ref}>
            <button
                onClick={() => { setOpen(!open); if (!open) markAllRead() }}
                className="relative p-2 rounded-lg hover:bg-surface-2 transition-colors"
            >
                <Bell className="w-4 h-4 text-muted" />
                {unread > 0 && (
                    <span className="absolute -top-0.5 -right-0.5 w-4 h-4 bg-bad rounded-full text-[9px] font-bold text-white flex items-center justify-center animate-pulse">
                        {unread > 9 ? '9+' : unread}
                    </span>
                )}
            </button>

            {open && (
                <div className="absolute right-0 top-10 w-[360px] bg-surface border border-border-2 rounded-xl shadow-2xl z-50 animate-in fade-in slide-in-from-top-2 duration-150">
                    <div className="flex items-center justify-between px-4 py-3 border-b border-border">
                        <span className="text-sm font-semibold">Notifications</span>
                        <button onClick={() => setOpen(false)} className="text-muted hover:text-[#e2e8f0]">
                            <X className="w-4 h-4" />
                        </button>
                    </div>
                    <div className="max-h-[400px] overflow-y-auto">
                        {notifications.length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted">No notifications</div>
                        )}
                        {notifications.map((n) => {
                            const Icon = ICONS[n.type]
                            return (
                                <div key={n.id} className={cn('px-4 py-3 border-b border-border/50 last:border-0', !n.read && 'bg-surface-2/50')}>
                                    <div className="flex items-start gap-2.5">
                                        <Icon className={`w-4 h-4 mt-0.5 shrink-0 ${COLORS[n.type]}`} />
                                        <div className="flex-1 min-w-0">
                                            <div className="text-sm font-medium">{n.title}</div>
                                            {n.message && <div className="text-xs text-muted mt-0.5 truncate">{n.message}</div>}
                                            <div className="text-[10px] text-muted-2 font-mono mt-1">{n.ts.toLocaleTimeString()}</div>
                                        </div>
                                    </div>
                                </div>
                            )
                        })}
                    </div>
                </div>
            )}
        </div>
    )
}
