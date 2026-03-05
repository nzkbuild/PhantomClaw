const BASE = ''

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
    const headers: Record<string, string> = { ...init?.headers as Record<string, string> }
    const method = init?.method?.toUpperCase() ?? 'GET'
    if (method !== 'GET' && method !== 'HEAD') {
        headers['Content-Type'] = headers['Content-Type'] ?? 'application/json'
    }
    const res = await fetch(`${BASE}${path}`, { ...init, headers })
    if (!res.ok) {
        const body = await res.text().catch(() => '')
        throw new Error(body || `${res.status} ${res.statusText}`)
    }
    return res.json()
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
    return api(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined })
}

/* ── Typed API helpers ── */

// Snapshot fields match backend main.go Snapshot dep output (flat map)
export type Snapshot = {
    mode: string
    session: string
    open_positions: number
    max_positions: number
    daily_loss: number
    max_daily_loss: number
    max_drawdown_pct: number
    halted: boolean
    time: string
    last_signal_time?: string
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

// All backend handlers return flat maps via writeJSON(w, result)
// No nesting like { snapshot: ... } — the fields ARE the response
export const getSnapshot = () => api<Snapshot>('/api/snapshot')
export const getDecisions = (limit = 20, symbol = '') =>
    api<{ count: number; decisions: Decision[] }>(`/api/decisions?limit=${limit}&symbol=${symbol}`)
export const getEquity = (days = 30) => api<{ days: number; points: unknown[] }>(`/api/equity?days=${days}`)
export const getAnalytics = (days = 7) => api<{ days: number; summary: unknown; pairs: unknown[] }>(`/api/analytics?days=${days}`)
export const getDiagnostics = () => api<Record<string, unknown>>('/api/diagnostics')
export const getLogs = (limit = 100) => api<{ count: number; logs: unknown[] }>(`/api/logs?limit=${limit}`)

// Usage returns { usage: [...], total_tokens, total_requests }
export const getUsage = () => api<{ usage: unknown[]; total_tokens: number; total_requests: number }>('/api/usage')

// Config returns { files: [{name, path, content}, ...] }
export const getConfig = () => api<{ files: { name: string; path: string; content: string }[] }>('/api/config')
export const saveConfig = (file: string, content: string) => apiPost<{ status: string }>('/api/config', { file, content })

// Sessions returns { sessions: [...] }
export const getSessionsList = () => api<{ sessions: unknown[] }>('/api/sessions/list')

// Risk returns { risk: { max_lot, max_daily_loss, current_daily_loss, ... } }
export const getRisk = () => api<{ risk: Record<string, unknown> }>('/api/risk')

// Cron
export const getCron = () => api<{ jobs: unknown[] }>('/api/cron')
export const fireCron = (id: string) => apiPost(`/api/cron/${id}/fire`)
export const toggleCron = (id: string) => apiPost(`/api/cron/${id}/toggle`)

export const switchMode = (mode: string) => apiPost('/api/mode', { mode })
export const switchModel = (name: string) => apiPost(`/api/switch-model?name=${name}`)
export const resetProvider = (name: string) => apiPost(`/api/provider/reset?name=${name}`)
export const sendChat = (message: string) => apiPost<{ reply: string }>('/api/chat', { message })
