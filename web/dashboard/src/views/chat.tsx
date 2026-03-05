import { useState, useRef, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Send, Bot, User } from 'lucide-react'

type Message = {
    role: 'user' | 'assistant'
    content: string
    ts: Date
}

export function ChatView() {
    const [messages, setMessages] = useState<Message[]>([])
    const [input, setInput] = useState('')
    const [loading, setLoading] = useState(false)
    const bottomRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
        bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }, [messages])

    const send = async () => {
        if (!input.trim() || loading) return
        const userMsg: Message = { role: 'user', content: input.trim(), ts: new Date() }
        setMessages((prev) => [...prev, userMsg])
        setInput('')
        setLoading(true)

        try {
            const res = await fetch('/api/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: userMsg.content }),
            })
            const data = await res.json()
            setMessages((prev) => [...prev, { role: 'assistant', content: data.reply || 'No response.', ts: new Date() }])
        } catch (e) {
            setMessages((prev) => [...prev, { role: 'assistant', content: `Error: ${e}`, ts: new Date() }])
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="flex flex-col h-[calc(100vh-56px)]">
            <div className="flex items-center justify-between mb-4">
                <h1 className="text-lg font-semibold">Chat</h1>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto space-y-3 pr-2">
                {messages.length === 0 && (
                    <div className="flex flex-col items-center justify-center h-full text-muted text-sm">
                        <Bot className="w-12 h-12 mb-3 text-muted-2" />
                        <p>Ask PhantomClaw anything about your trades, strategy, or performance.</p>
                    </div>
                )}
                {messages.map((msg, i) => (
                    <div key={i} className={`flex gap-3 ${msg.role === 'user' ? 'justify-end' : ''}`}>
                        {msg.role === 'assistant' && (
                            <div className="w-7 h-7 rounded-full bg-amber/10 flex items-center justify-center shrink-0 mt-0.5">
                                <Bot className="w-4 h-4 text-amber" />
                            </div>
                        )}
                        <Card className={`max-w-[70%] px-4 py-3 text-sm leading-relaxed ${msg.role === 'user'
                                ? 'bg-surface-3 border-border-2 text-[#e2e8f0]'
                                : 'bg-surface border-border'
                            }`}>
                            <p className="whitespace-pre-wrap">{msg.content}</p>
                        </Card>
                        {msg.role === 'user' && (
                            <div className="w-7 h-7 rounded-full bg-surface-3 flex items-center justify-center shrink-0 mt-0.5">
                                <User className="w-4 h-4 text-muted" />
                            </div>
                        )}
                    </div>
                ))}
                {loading && (
                    <div className="flex gap-3">
                        <div className="w-7 h-7 rounded-full bg-amber/10 flex items-center justify-center shrink-0">
                            <Bot className="w-4 h-4 text-amber animate-pulse" />
                        </div>
                        <Card className="bg-surface border-border px-4 py-3">
                            <span className="text-sm text-muted animate-pulse">Thinking...</span>
                        </Card>
                    </div>
                )}
                <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className="mt-4 flex gap-2">
                <input
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && !e.shiftKey && send()}
                    placeholder="Ask about trades, P&L, strategy..."
                    disabled={loading}
                    className="flex-1 bg-surface-2 border border-border-2 rounded-lg px-4 py-2.5 text-sm outline-none focus:border-amber transition-colors placeholder:text-muted-2"
                />
                <Button onClick={send} disabled={loading || !input.trim()} size="icon" className="bg-amber hover:bg-amber-dim text-background">
                    <Send className="w-4 h-4" />
                </Button>
            </div>
        </div>
    )
}
