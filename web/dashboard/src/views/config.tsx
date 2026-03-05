import { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Save, FileCode, RefreshCw } from 'lucide-react'
import { api, apiPost } from '@/lib/api'
import { useApi } from '@/hooks/use-data'

type ConfigFile = {
    name: string
    path: string
    content: string
}

export function ConfigView() {
    const [activeFile, setActiveFile] = useState<'config' | 'soul'>('config')
    const [content, setContent] = useState('')
    const [saving, setSaving] = useState(false)
    const [saveMsg, setSaveMsg] = useState('')

    const { loading, refetch } = useApi(
        () => api<{ files: ConfigFile[] }>('/api/config').then((r) => {
            const file = r.files?.find((f) => f.name === (activeFile === 'config' ? 'config.yaml' : 'soul.md'))
            if (file) setContent(file.content)
            return r
        }),
        [activeFile]
    )

    const handleSave = async () => {
        setSaving(true)
        setSaveMsg('')
        try {
            await apiPost('/api/config', {
                file: activeFile === 'config' ? 'config.yaml' : 'soul.md',
                content,
            })
            setSaveMsg('Saved successfully')
            setTimeout(() => setSaveMsg(''), 3000)
        } catch (e) {
            setSaveMsg(`Error: ${e}`)
        } finally {
            setSaving(false)
        }
    }

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-lg font-semibold">Config Editor</h1>
                <div className="flex gap-2">
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={() => refetch()}
                        className="border-border text-muted hover:text-[#e2e8f0]"
                    >
                        <RefreshCw className="w-3 h-3 mr-1" /> Reload
                    </Button>
                    <Button
                        size="sm"
                        onClick={handleSave}
                        disabled={saving}
                        className="bg-amber hover:bg-amber-dim text-background"
                    >
                        <Save className="w-3 h-3 mr-1" /> Save
                    </Button>
                </div>
            </div>

            {saveMsg && (
                <Badge variant="outline" className={`font-mono text-xs ${saveMsg.includes('Error') ? 'text-bad border-bad-dim' : 'text-ok border-ok-dim'}`}>
                    {saveMsg}
                </Badge>
            )}

            {/* File tabs */}
            <div className="flex gap-2">
                {(['config', 'soul'] as const).map((tab) => (
                    <button
                        key={tab}
                        onClick={() => setActiveFile(tab)}
                        className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${activeFile === tab
                                ? 'bg-amber/10 text-amber border border-amber-dim'
                                : 'bg-surface-2 text-muted border border-border hover:text-[#e2e8f0]'
                            }`}
                    >
                        <FileCode className="w-3 h-3" />
                        {tab === 'config' ? 'config.yaml' : 'soul.md'}
                    </button>
                ))}
            </div>

            {loading && <div className="text-muted text-sm animate-pulse">Loading...</div>}

            <Card className="bg-surface border-border">
                <CardContent className="p-0">
                    <textarea
                        value={content}
                        onChange={(e) => setContent(e.target.value)}
                        spellCheck={false}
                        className="w-full h-[calc(100vh-280px)] bg-transparent p-4 font-mono text-[12px] leading-relaxed text-[#e2e8f0] outline-none resize-none"
                        placeholder="Loading file..."
                    />
                </CardContent>
            </Card>
        </div>
    )
}
