import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'

type Theme = 'dark' | 'light'
type Accent = 'amber' | 'cyan' | 'violet' | 'rose' | 'emerald'

type ThemeCtx = {
    theme: Theme
    accent: Accent
    setTheme: (t: Theme) => void
    setAccent: (a: Accent) => void
}

const ThemeContext = createContext<ThemeCtx>({
    theme: 'dark',
    accent: 'amber',
    setTheme: () => { },
    setAccent: () => { },
})

export function useTheme() {
    return useContext(ThemeContext)
}

export function ThemeProvider({ children }: { children: ReactNode }) {
    const [theme, setThemeState] = useState<Theme>(() => {
        return (localStorage.getItem('phantom-theme') as Theme) || 'dark'
    })
    const [accent, setAccentState] = useState<Accent>(() => {
        return (localStorage.getItem('phantom-accent') as Accent) || 'amber'
    })

    const setTheme = useCallback((t: Theme) => {
        setThemeState(t)
        localStorage.setItem('phantom-theme', t)
        document.documentElement.setAttribute('data-theme', t)
    }, [])

    const setAccent = useCallback((a: Accent) => {
        setAccentState(a)
        localStorage.setItem('phantom-accent', a)
        document.documentElement.setAttribute('data-accent', a)
    }, [])

    return (
        <ThemeContext.Provider value={{ theme, accent, setTheme, setAccent }}>
            {children}
        </ThemeContext.Provider>
    )
}

const ACCENTS: { id: Accent; color: string; label: string }[] = [
    { id: 'amber', color: '#f5a623', label: 'Amber' },
    { id: 'cyan', color: '#00d4ff', label: 'Cyan' },
    { id: 'violet', color: '#8b5cf6', label: 'Violet' },
    { id: 'rose', color: '#f43f5e', label: 'Rose' },
    { id: 'emerald', color: '#10b981', label: 'Emerald' },
]

export function ThemeSwitcher() {
    const { theme, accent, setTheme, setAccent } = useTheme()

    return (
        <div className="space-y-4">
            <div>
                <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Theme</div>
                <div className="flex gap-2">
                    {(['dark', 'light'] as Theme[]).map((t) => (
                        <button
                            key={t}
                            onClick={() => setTheme(t)}
                            className={`px-3 py-1.5 rounded-md text-xs font-medium border transition-colors ${theme === t
                                    ? 'bg-amber/10 text-amber border-amber-dim'
                                    : 'bg-surface-2 text-muted border-border hover:text-[#e2e8f0]'
                                }`}
                        >
                            {t.charAt(0).toUpperCase() + t.slice(1)}
                        </button>
                    ))}
                </div>
            </div>
            <div>
                <div className="text-[10px] font-semibold tracking-[.12em] uppercase text-muted mb-2">Accent</div>
                <div className="flex gap-2">
                    {ACCENTS.map((a) => (
                        <button
                            key={a.id}
                            onClick={() => setAccent(a.id)}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium border transition-colors ${accent === a.id ? 'border-[var(--color-amber)]' : 'border-border hover:border-border-2'
                                }`}
                        >
                            <div className="w-3 h-3 rounded-full" style={{ background: a.color }} />
                            {a.label}
                        </button>
                    ))}
                </div>
            </div>
        </div>
    )
}
