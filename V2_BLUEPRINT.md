# PhantomClaw v2 — Blueprint

> Everything we learned from OpenClaw, ZeroClaw, and what PhantomClaw needs next.

---

## Where We Stand Today

PhantomClaw v1.0 is a **trading specialist** — it has things OpenClaw will never have: MT5 bridge, risk engine, confidence scoring, correlation guards, spread filters, strategy versioning, and a self-learning diary. **That stays.**

But as a **general agent framework**, we're behind. OpenClaw has 22 LLM providers, autonomous scheduling, conversation memory, structured tool calling, and an event-driven architecture. We have 3 providers and a reactive one-shot loop.

---

## Data Flow: Today vs Target

```
TODAY:
  EA signal → buildPrompt (one-shot) → LLM → parse text → trade or hold → done
  (no memory of what it said 15 min ago)

TARGET (v2):
  EA signal ─┐
  Cron job ──┤→ inject (history + tools + strategy + market) → LLM → tool calls → loop → output
  Telegram ──┤                                                                        ↓
  Webhook ───┘                                                              JSONL store + memory + diary
```

---

## The 4 Pillars We Need

### Pillar 1: Provider Layer ⚡
**Problem:** 3 hardcoded providers (Claude, GPT-4o, DeepSeek). Adding one = writing a new Go file.
**OpenClaw pattern:** `provider/model` format + generic OpenAI-compatible adapter + error classifier + cooldown.

**What's happening in plain English:**
> Right now, if you want to add a new AI brain (like Groq or Ollama), I have to write a whole new Go file. That's dumb. OpenClaw solved this — most AI providers speak the same language (OpenAI format). So we build ONE adapter that works with any of them. You just add a name, URL, and API key to config.yaml = done.
>
> We also need the bot to be SMART about failures. If Claude goes down at 3 AM, the bot shouldn't just blindly try the next one — it should know WHY it failed (rate limit? bad key? model doesn't exist?) and react differently. And if a provider keeps failing, put it in timeout so we stop wasting time on it.

| Provider | Format | Status |
|----------|--------|--------|
| Claude | `anthropic/claude-sonnet-4` | ✅ Have |
| GPT-4o | `openai/gpt-4o` | ✅ Have |
| DeepSeek | `deepseek/deepseek-chat` | ✅ Have |
| OpenRouter | `openrouter/meta-llama/llama-3.1-405b` | ❌ Need generic adapter |
| Ollama | `ollama/qwen3` | ❌ Need generic adapter |
| Groq | `groq/llama-3.3-70b` | ❌ Need generic adapter |
| Gemini | `google/gemini-2.5-flash` | ❌ Need dedicated (different API) |
| Mistral | `mistral/mistral-large` | ❌ Need generic adapter |

> **Key insight:** OpenRouter, Ollama, Groq, Mistral, Together AI all speak OpenAI format. **One generic adapter = all of them.**

**Components to build:**
```
Provider Registry
  ├── GenericProvider (base_url + api_key + model) ← covers 80% of providers
  ├── ClaudeProvider (Anthropic's non-standard API) ← already have
  └── GeminiProvider (Google's non-standard API) ← future

Execution Router (replaces current basic Router)
  ├── Primary → fallback chain (same as now, but smarter)
  ├── Error Classifier
  │     ├── rate_limit → wait & retry same provider
  │     ├── auth_error → skip, alert on Telegram
  │     ├── model_not_found → skip, log warning
  │     ├── network_error → retry once, then fallback
  │     └── unknown → fallback immediately
  ├── Per-provider Cooldown
  │     └── 3 failures in 5 min → cooldown 5 min → auto-recover
  └── Model Aliases
        ├── "fast" → groq/llama-3.3-70b
        ├── "cheap" → deepseek/deepseek-chat
        ├── "smart" → anthropic/claude-sonnet-4
        └── "local" → ollama/qwen3
```

**Effort:** Small-Medium · **Impact:** 🔴 Critical

---

### Pillar 2: Conversation Memory 🧠
**Problem:** Each EA signal is a blank slate. Agent forgets what it said 15 minutes ago.
**OpenClaw pattern:** JSONL session store. Every turn saved, fed back on next call.

**What to build:**
- JSONL file per trading day (e.g., `sessions/2026-03-01.jsonl`)
- Each entry: `{role, content, timestamp, pair, decision}`
- Inject last N turns into prompt (sliding window, ~2000 tokens)
- Auto-rotate at midnight

**Effort:** Small · **Impact:** 🔴 Critical

---

### Pillar 3: Heartbeat & Cron 💓
**Problem:** Agent only wakes when EA sends a signal. Can't think proactively.
**OpenClaw pattern:** `cron.add` / `cron.wake` — agent creates its own schedules.

**What to build:**
- **System cron:** Morning scan (08:00), pre-London alert (14:45), position check (every 30 min during trading)
- **Agent cron:** Agent can tell the system "wake me in 30 minutes to recheck EUR/USD"
- **Heartbeat ping:** Every 5 min → health check → Telegram if something's wrong

**Effort:** Medium · **Impact:** 🟡 High

---

### Pillar 4: Native Tool Calling 🔧
**Problem:** We parse text output ("PLACE_PENDING: ..."). Fragile, can't multi-step.
**OpenClaw pattern:** Structured JSON tool calls. Model outputs `{tool: "place_pending", args: {...}}`. System executes, feeds result back, model thinks again.

**What to build:**
- Use LLM's native function calling (Claude tool_use, OpenAI function_calling)
- Tool schemas with human-readable descriptions in system prompt
- ReAct loop: think → call tool → observe result → think again → call another tool
- Loop detection: max 5 iterations, token budget check
- Tool profiles: `trading` (get_price, place_pending, get_positions) vs `readonly` (get_price only)

**Effort:** Medium · **Impact:** 🟡 High

---

## Phase Plan

### Phase A — Provider Layer (1-2 sessions)
- [ ] Generic `openai_compat.go` provider (base URL + API key + model)
- [ ] `provider/model` config format in `config.yaml`
- [ ] Error classifier (rate_limit / auth / model_not_found / network)
- [ ] Per-provider cooldown (3 fails → 5 min timeout → auto-recover)
- [ ] Model aliases in config (`fast`, `cheap`, `smart`, `local`)
- [ ] Refactor router to use new format
- [ ] Add config entries for: OpenRouter, Ollama, Groq
- [ ] Test with at least 2 new providers

### Phase B — Agent Intelligence (2-3 sessions)
- [ ] JSONL session store (per-day conversation history)
- [ ] Inject last N turns into `buildPrompt()`
- [ ] System cron jobs (morning scan, pre-London, position checks)
- [ ] Agent-initiated cron (`cron.add` tool)
- [ ] Heartbeat ping to Telegram
- [ ] Tool documentation layer in system prompt

### Phase C — Tool System (2-3 sessions)
- [ ] Native tool calling (Claude tool_use / OpenAI function_calling)
- [ ] ReAct loop (multi-step tool chains)
- [ ] Loop detection guardrails
- [ ] Tool profiles (`trading`, `readonly`, `full`)
- [ ] Per-provider tool restrictions
- [ ] New tools: `web_search`, `web_fetch` (market research)

---

## What We Keep (Our Edge)

These are things OpenClaw **doesn't have** and we shouldn't touch:

| Feature | Why It's Our Edge |
|---------|------------------|
| MT5 bridge | Direct EA communication — no other agent has this |
| Risk engine | Hard limits the AI can never override |
| Confidence scoring | 7-factor gate before any trade |
| Correlation guard | Blocks conflicting pair exposure |
| Spread filter | Rejects abnormal market conditions |
| Strategy versioning | AI evolves its own trading playbook |
| Session scheduler (MYT) | Purpose-built for our trading hours |
| Post-mortem learning | AI writes lessons after every trade |
| Ramp-up system | Starts cautious, grows with proof |

---

## Priority Matrix

| Item | Effort | Impact | Do When |
|------|--------|--------|---------|
| Generic OpenAI-compat provider | Small | 🔴 Critical | Phase A |
| OpenRouter / Ollama config | Tiny | 🔴 Critical | Phase A |
| JSONL conversation history | Small | 🔴 Critical | Phase B |
| System cron (morning scan, etc.) | Medium | 🟡 High | Phase B |
| Native tool calling + ReAct loop | Medium | 🟡 High | Phase C |
| Agent-initiated cron | Medium | 🟡 High | Phase B |
| Tool profiles + restrictions | Medium | 🟢 Medium | Phase C |
| Vector memory / semantic search | Medium | 🟢 Medium | Future |
| Vision / chart analysis | Large | 🟢 Medium | Future |
| Web dashboard | Large | ⚪ Low | Future |
