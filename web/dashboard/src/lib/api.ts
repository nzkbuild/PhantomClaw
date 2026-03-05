const BASE = ''

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(`${BASE}${path}`, {
        ...init,
        headers: { 'Content-Type': 'application/json', ...init?.headers },
    })
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
    return res.json()
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
    return api(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined })
}

/* ── Typed API helpers ── */

export type Snapshot = {
    mode: string
    session: string
    open_positions: number
    max_positions: number
    daily_loss: number
    daily_pnl: number
    win_rate_7d: number
    total_trades_7d: number
    uptime: string
    provider: string
    provider_status: Record<string, string>
}

export type Decision = {
    id: number
    symbol: string
    decision: string
    reason: string
    created_at: string
}

export type ProviderInfo = {
    name: string
    model: string
    status: string
    is_primary: boolean
}

export const getSnapshot = () => api<{ snapshot: Snapshot }>('/api/snapshot')
export const getDecisions = (limit = 20, symbol = '') =>
    api<{ decisions: Decision[] }>(`/api/decisions?limit=${limit}&symbol=${symbol}`)
export const getEquity = (days = 30) => api<{ equity: unknown[] }>(`/api/equity?days=${days}`)
export const getAnalytics = (days = 7) => api<{ analytics: unknown }>(`/api/analytics?days=${days}`)
export const getDiagnostics = () => api<{ diagnostics: unknown }>('/api/diagnostics')
export const getLogs = (limit = 100) => api<{ logs: unknown[] }>(`/api/logs?limit=${limit}`)

export const switchMode = (mode: string) => apiPost('/api/mode', { mode })
export const switchModel = (name: string) => apiPost(`/api/switch-model?name=${name}`)
export const resetProvider = (name: string) => apiPost(`/api/provider/reset?name=${name}`)
export const sendChat = (message: string) => apiPost<{ reply: string }>('/api/chat', { message })
