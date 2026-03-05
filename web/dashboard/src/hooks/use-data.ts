import { useEffect, useRef, useCallback, useState } from 'react'

type SSEOptions = {
    onSnapshot?: (data: unknown) => void
    onLog?: (data: unknown) => void
    onNotification?: (data: unknown) => void
}

export function useSSE(url: string, options: SSEOptions) {
    const optionsRef = useRef(options)
    optionsRef.current = options

    useEffect(() => {
        const es = new EventSource(url)

        es.addEventListener('snapshot', (e) => {
            try {
                const data = JSON.parse(e.data)
                optionsRef.current.onSnapshot?.(data)
            } catch { /* ignore parse errors */ }
        })

        es.addEventListener('logs', (e) => {
            try {
                const data = JSON.parse(e.data)
                optionsRef.current.onLog?.(data)
            } catch { /* ignore */ }
        })

        es.addEventListener('notification', (e) => {
            try {
                const data = JSON.parse(e.data)
                optionsRef.current.onNotification?.(data)
            } catch { /* ignore */ }
        })

        es.onerror = () => {
            // Auto-reconnect handled by EventSource
        }

        return () => es.close()
    }, [url])
}

export function useInterval(callback: () => void, delay: number) {
    const savedCallback = useRef(callback)
    savedCallback.current = callback

    useEffect(() => {
        const tick = () => savedCallback.current()
        const id = setInterval(tick, delay)
        return () => clearInterval(id)
    }, [delay])
}

export function useApi<T>(fetcher: () => Promise<T>, deps: unknown[] = []) {
    const [data, setData] = useState<T | null>(null)
    const [error, setError] = useState<string | null>(null)
    const [loading, setLoading] = useState(true)

    const refetch = useCallback(() => {
        setLoading(true)
        setError(null)
        fetcher()
            .then(setData)
            .catch((e: Error) => setError(e.message))
            .finally(() => setLoading(false))
    }, deps) // eslint-disable-line react-hooks/exhaustive-deps

    useEffect(() => { refetch() }, [refetch])

    return { data, error, loading, refetch }
}
