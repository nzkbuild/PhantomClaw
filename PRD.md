# PhantomClaw — Product Requirements Document (PRD)
> Version: 1.0 — Final  
> Date: 2026-02-27  
> Status: Ready for approval

---

## 1. What Is PhantomClaw?

PhantomClaw is a **personal autonomous AI trading agent** — a specialized, self-hosted system that:

- Thinks and reasons using LLMs (Claude, GPT-4o, Gemini, DeepSeek)
- Communicates with you exclusively via **Telegram**
- Executes trades on **MetaTrader 5** through an EA REST bridge
- Learns from every trade, self-corrects, and evolves its strategy over time
- Adapts its trading style across timeframes — not locked to one approach
- Operates on **MYT (UTC+8)** with session-aware trading windows
- Runs 24/7 on a **Windows VPS** as a background service

It is not a general-purpose assistant. It is purpose-built for trading intelligence.

---

## 2. Core Principles

| Principle | Meaning |
|---|---|
| **Telegram-first** | Every human interaction happens through Telegram. No web UI, no dashboard. |
| **Autonomous by default** | Default mode is `AUTO` — bot trades within hard risk limits. `SUGGEST` available as fallback. |
| **Composable architecture** | Every layer (LLM, memory, tools, channels) is swappable via config. |
| **Minimal external deps** | SQLite for memory. No cloud DBs, no Redis, no Kafka. |
| **You own everything** | Runs on your VPS, your broker, your API keys. No third-party cloud dependency. |

---

## 3. Project Name & Repository

- **Project name**: `PhantomClaw`
- **GitHub repo**: `PhantomClaw` (existing — your coding folder)
- **Primary language**: Go
- **VPS**: Windows Server 2022 Datacenter 64-bit EN

---

## 4. System Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    YOU (via Telegram)                    │
│        Commands: /status /mode /halt /report /ask       │
└──────────────────────────┬──────────────────────────────┘
                           │ Telegram Bot API
┌──────────────────────────▼──────────────────────────────┐
│               PHANTOMCLAW CORE (Go Binary)               │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Telegram     │  │ LLM Adapter  │  │ Safety Engine │  │
│  │ Handler      │  │ (multi-prov) │  │ + Risk Guards │  │
│  └──────────────┘  └──────────────┘  └───────────────┘  │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Skills /     │  │ Memory       │  │ Trade Journal │  │
│  │ Tool Dispatch│  │ (SQLite)     │  │ + Learner     │  │
│  └──────────────┘  └──────────────┘  └───────────────┘  │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐                      │
│  │ Market Data  │  │ REST Bridge  │  (HTTP server)        │
│  │ Connectors   │  │ for MT5 EA   │                      │
│  └──────────────┘  └──────────────┘                      │
└──────────────────────────────────────────────────────────┘
         │ HTTP REST (localhost)        │ External APIs
         ▼                              ▼
┌─────────────────┐          ┌─────────────────────────┐
│  MT5 Terminal   │          │  Market Data APIs        │
│  (EA + MQL5)    │          │  News / Sentiment / FX   │
│  WebRequest()   │          └─────────────────────────┘
└─────────────────┘
```

---

## 5. Deployment Topology

**Single Windows VPS** (Windows Server 2022 Datacenter 64-bit):

| Component | Location | Notes |
|---|---|---|
| MT5 Terminal + EA | `C:\Program Files\MetaTrader 5\` | Installed normally |
| PhantomClaw binary | `C:\PhantomClaw\phantomclaw.exe` | Go compiled for Windows amd64 |
| Config file | `C:\PhantomClaw\config.yaml` | All settings, no hardcoding |
| SQLite database | `C:\PhantomClaw\data\phantom.db` | Trade memory, lessons, state |
| Log files | `C:\PhantomClaw\logs\` | Daily rotating JSON logs |

EA ↔ Bot communication: **localhost only** (`http://127.0.0.1:PORT`)  
No public port needed. No SSL needed for EA bridge (internal traffic).

### EA Chart Attachment — How Many Charts?

**One EA per symbol, attached to M1.** That's it.

MQL5 allows an EA on any chart to pull data from **any timeframe on any symbol** natively:

```mql5
// From a single XAUUSD M1 chart, EA collects the full MTF stack:
double h4  = iClose("XAUUSD", PERIOD_H4, 0);
double d1  = iClose("XAUUSD", PERIOD_D1, 0);
double h1  = iClose("XAUUSD", PERIOD_H1, 0);
```

The EA sends **one HTTP POST** to the Go bot containing all TF data. Go bot never needs to know which chart the EA is attached to.

**For a 4-pair watchlist = 4 chart tabs, 4 EA instances:**
```
XAUUSD M1  →  EA sends: M5, M15, H1, H4, D1 in one payload
EURUSD M1  →  same
USDJPY M1  →  same
GBPUSD M1  →  same
```
Not 20 charts. Not multiple attachments per pair.

---

## 6. MYT Session Windows (UTC+8)

PhantomClaw's entire schedule is anchored to **Malaysia Time (MYT = UTC+8)**:

| Window | MYT | UTC | Mode | Activity |
|---|---|---|---|---|
| **Off hours** | 00:00–08:00 | 16:00–00:00 | `LEARNING` | RAG rebuild, nightly review, tomorrow's prep, DB housekeeping |
| **Tokyo Open** | 08:00–15:00 | 00:00–07:00 | `RESEARCH` | Full data refresh, news, MTF pre-analysis, economic calendar |
| **London Open** | 15:00–20:00 | 07:00–12:00 | `TRADING` | Live signals, SUGGEST/AUTO execution |
| **NY/London Overlap** | 20:00–00:00 | 12:00–16:00 | `TRADING` | Peak liquidity — highest signal confidence |
| **Hard stop** | 00:00 MYT | 16:00 UTC | — | No new entries. Manage open trades to close. |

> **Why this works**: Tokyo collects overnight moves. London acts. NY confirms or reverses. Off hours are not idle — the bot gets smarter while you sleep.

### LEARNING Mode (00:00–08:00 MYT) — What the Bot Does While You Sleep

| Task | Description |
|---|---|
| **Deep RAG build** | Re-index all trade lessons, compress memory, re-rank by relevance score |
| **Nightly strategy review** | LLM reviews last 7 days of trades → rewrites strategy bias document |
| **Tomorrow's prep** | Pre-computes MTF bias for all pairs ahead of London open |
| **Sydney pair monitoring** | Watches AUDUSD, NZDUSD, USDJPY — logs patterns, no execution |
| **Knowledge crawl** | Pulls macro/news from overnight US sessions, stores with TTL |
| **Simulation run** | Replays today's signals vs actual outcomes → updates learner weights |
| **Self-performance audit** | Recalculates per-pair, per-session win rate → updates adaptive weights |
| **DB housekeeping** | VACUUM SQLite, rotate logs, prune stale cache entries |

---

## 7. Adaptive Trading Style

PhantomClaw does **not** lock to one timeframe. It uses a **Multi-Timeframe (MTF) confirmation stack** to find stable setups:

```
Trend context:   D1 / W1    → Is the higher trend bullish or bearish?
Swing anchor:    4H         → Where is the setup forming?
Entry timing:    1H / 15M   → Precise entry trigger
Execution:       5M / 1M    → Fine-tune entry, spread-awareness
```

**Style target: Stable Intraday (between scalp and swing)**
- Hold time: 2–12 hours typical
- Trades per day: 1–3 maximum (quality over quantity)
- Avoids scalping (human latency incompatible) and pure swing (overnight risk)
- Bot dynamically adjusts confidence threshold based on session liquidity

**Adaptive mechanism:**
- Bot tracks its own win rate per timeframe, per session, per pair
- After 20 trades, it weights signals higher from the combos with best historical performance
- This is the "self-evolve" — not neural retraining, but **strategy parameter self-tuning via memory**

> **Acknowledged flaw**: Even the pre-analysis cache has a weakness — if a major news event fires between cache refresh and signal, the cached analysis is stale. **Fix**: cache TTL is reduced to 15 minutes during London/NY sessions, and any high-impact news event (from economic calendar) automatically triggers an immediate re-analysis cycle.

---

## 8. Trade Execution Flow

**Hybrid Push/Pull** — bot runs itself with minimal supervision:

### A. Background Pre-Analysis (always running during trading hours)
```
Every 15 minutes during London/NY session:
  → Pull fresh OHLCV for all watched pairs (all TFs: 5M, 15M, 1H, 4H, D1)
  → Check economic calendar for upcoming events
  → Run LLM analysis ASYNC — result stored in SQLite cache
  → Cache tagged with freshness timestamp
  
Result: when EA fires a signal, analysis is ALREADY done.
Decision in <100ms instead of 2–8 seconds.
```

### B. Pending Order Strategy (Primary Execution Method — Zero Lag)

The core loophole that eliminates execution lag entirely:

```
Instead of reacting to price (market orders with slippage),
PhantomClaw pre-places pending orders at calculated key levels.
MT5 fills them automatically — zero bot involvement at fill time.
```

| Order Type | When used | Lag exposure |
|---|---|---|
| `BUY LIMIT` | Anticipating pullback to support | ✅ Zero — fills at your exact price |
| `SELL LIMIT` | Anticipating rejection at resistance | ✅ Zero — fills at your exact price |
| `BUY STOP` | Breakout above key level | ✅ Near-zero — fills on momentum |
| `SELL STOP` | Breakdown below key level | ✅ Near-zero — fills on momentum |
| `MARKET` | Emergency HALT / forced close only | ⚠️ Minor slippage possible |

**How it works in practice:**
```
During RESEARCH mode (08:00–15:00 MYT):
  → Bot calculates key S/R levels for each pair (MTF analysis)
  → Identifies ideal pending order levels for today's session
  → Places pending orders in MT5 via EA before London opens
  → Orders sit at pre-calculated levels, MT5 fills when price arrives

During TRADING mode (15:00–00:00 MYT):
  → New signals may add/modify pending orders
  → Live candle data validates or cancels stale pending orders
  → If level is no longer valid → EA cancels pending order
```

**Result**: Bot is fully autonomous. Fills happen at the bot's pre-determined price. No market order chasing. No lag. No slippage.

### C. EA Push (Event-driven, signal-triggered)
```
MT5 candle close / threshold breach
  → EA calls POST http://127.0.0.1:8765/signal (OHLCV + indicators)
  → EA timeout: 500ms hard cap (returns HOLD if bot overruns)
  → Bot reads pre-cached analysis from SQLite (~5ms)
  → Applies risk guardrails (~2ms)
  → AUTO mode: responds with one of:
      PLACE_PENDING {type, symbol, level, lot, sl, tp}
      MODIFY_PENDING {ticket, new_sl, new_tp}
      CANCEL_PENDING {ticket}
      MARKET_CLOSE {ticket}  ← only for emergency/HALT
      HOLD
  → EA executes the instruction and confirms back
  → Telegram notification sent (async — doesn't block execution)
```

### C. Bot Pull (Scheduled research)
```
08:00 MYT (Tokyo open):
  → Full data refresh: news, sentiment, economic calendar, macro
  → LLM morning brief: "Today I'm watching..."
  → Telegram digest sent to you

14:45 MYT (15min before London open):
  → Pre-session analysis: MTF bias per pair
  → Telegram alert: "London opens in 15min. Setup on XAUUSD: bullish 4H."
```

### D. Self-Correction (Post-trade)
```
Trade closed (profit or loss)
  → EA pushes result to POST /trade-result
  → Bot runs LLM post-mortem (async, non-blocking)
  → Writes lesson + strategy weight adjustment to SQLite
  → After every 10 trades: Telegram weekly-style mini-report
```

---

## 9. Safety Modes

| Mode | Bot Behavior | Command |
|---|---|---|
| `OBSERVE` | Analyzes and reports only. Zero execution. | `/mode observe` |
| `SUGGEST` | Proposes trade → waits for your ✅ via Telegram | `/mode suggest` |
| `AUTO` ⭐ **default** | Executes autonomously within hard risk limits | `/mode auto` |
| `HALT` 🛑 | Closes all positions, freezes everything | `/halt` |

**Why AUTO is safe as default:**
- Hard risk limits in Go code — LLM **cannot** override them under any condition
- Primary order type is **pending orders** (LIMIT/STOP) — fills at pre-calculated levels, not at reaction time
- Pre-analysis cache means decisions are made during RESEARCH, not during execution
- All actions logged + Telegram notifications sent after every execution
- HALT overrides everything instantly from Telegram

**SUGGEST remains available** as an optional mode you can switch to anytime — useful during:
- First week of live trading (building trust in the bot)
- Unusual market conditions you want to review manually
- After a strategy patch rollback

---

## 10. Risk Guardrails (Balanced Defaults)

These are **hard limits in Go code** — the LLM cannot override them:

| Guardrail | Default Value | Configurable |
|---|---|---|
| Max lot size per trade | `0.10 lots` | ✅ in config.yaml |
| Max daily loss (USD) | `$100` | ✅ |
| Max concurrent open positions | `3` | ✅ |
| Max drawdown before HALT | `10% of account equity` | ✅ |
| SUGGEST timeout | `3 minutes` | ✅ |
| Min time between trades | `15 minutes` (no overtrading) | ✅ |
| Trade size ramp-up | Start at 50% lot until 10 profitable trades | ✅ |

**TOS safety wrappers (broker + MT5):**
- WebRequest rate limiting (max 10 requests/minute to local endpoint)
- Proper `User-Agent` headers on all outbound HTTP calls
- No EA manipulation of broker SL/TP beyond account limits
- All actions logged with timestamps for audit

---

## 11. Go Library Composition Stack

> This is what PhantomClaw is **built from** — not a fork, but composed from these battle-tested Go libraries:

| Role | Library | GitHub | Why |
|---|---|---|---|
| **Agent brain** | `cloudwego/eino` | cloudwego/eino | ReAct loop, tool dispatch, LLM routing, memory — all in Go. 9.8k ⭐ |
| **Telegram** | `go-telegram/bot` | go-telegram/bot | Modern, clean Bot API wrapper for Go |
| **OpenAI / DeepSeek** | `sashabaranov/go-openai` | go-openai | OpenAI + any OpenAI-compatible endpoint (DeepSeek uses same API) |
| **Claude** | `anthropics/anthropic-sdk-go` | anthropic-sdk-go | Official Anthropic Go SDK |
| **Gemini** | `google.golang.org/genai` | google genai | Official Google GenAI Go SDK (GA as of 2025) |
| **SQLite** | `mattn/go-sqlite3` | go-sqlite3 | Battle-tested SQLite driver for Go |
| **Config** | `spf13/viper` | viper | YAML config loading, env var override |
| **Logging** | `uber-go/zap` | zap | Structured JSON logging, zero allocation |
| **Scheduling** | `robfig/cron` | cron | Session-aware scheduled tasks (Tokyo open, London open) |
| **HTTP server** | Go stdlib `net/http` | built-in | MT5 EA REST bridge (no framework needed) |

> All libraries are **MIT/Apache licensed**. No viral licenses, no commercial restrictions.

---

## 12. LLM Provider Layer

### Adapter Interface (Go)
Single interface, multiple implementations — swap via config with zero code change:

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error
    ToolCall(ctx context.Context, messages []Message, tools []Tool) (ToolResult, error)
}
```

### Supported Providers

| Provider | SDK | Notes |
|---|---|---|
| **Claude (Anthropic)** | `anthropics/anthropic-sdk-go` | Primary — best reasoning chains |
| **GPT-4o (OpenAI)** | `openai/openai-go` | Fallback + tool calling |
| **Gemini (Google)** | `google.golang.org/genai` | Free tier for research tasks |
| **DeepSeek** | OpenAI-compatible API | Cheap, fast, good for analysis |

### On "OAuth / Account Connection" (like OpenClaw)

> You asked about connecting via your ChatGPT Plus account instead of API tokens.

**Honest answer:**
- ChatGPT Plus (subscription) and OpenAI API are **separate billing systems**. Plus doesn't grant API access.
- What OpenClaw does is use **Google/GitHub OAuth** to authenticate *to OpenClaw's own service* — not to bypass OpenAI API billing.
- **What we CAN do**: Support `OPENAI_API_KEY` + `ANTHROPIC_API_KEY` via environment variables or config. For **Gemini**, your Google Account OAuth is possible via `google.golang.org/oauth2` — bringing your own Google login.
- **DeepSeek** is very cheap (~$0.14/M tokens) — likely your cost-saving LLM.

**Recommended**: Claude as primary (reasoning), DeepSeek as secondary (research/cost), Gemini for free market sentiment scans.

---

## 13. Memory Architecture (MemoryCore-Inspired)

> Adapted from Project-AI-MemoryCore patterns into PhantomClaw's SQLite + Go runtime.  
> Not markdown files — structured, queryable, fast. But the same layered memory philosophy.

---

### 13.1 Master Strategy Document

**The single source of truth the LLM loads first on every reasoning cycle.**

Rebuilt every LEARNING cycle (00:00–08:00 MYT) and stored as a row in SQLite + exported as `master-strategy.md` for human readability:

```
master-strategy.md
├── Current market bias per pair (bullish / bearish / neutral)
├── Win rate stats: last 7 days, per pair, per session window
├── Top 5 most relevant lessons from recent trades
├── Active strategy params (preferred setups, conditions to avoid)
├── Tomorrow's watchlist with pre-computed MTF bias
└── Strategy version + last updated timestamp
```

This is the first thing injected into every LLM prompt. Compact. Current. Always fresh.

---

### 13.2 SQLite Schema — Full Table Definitions

```sql
-- Core trade record
CREATE TABLE trades (
    id          INTEGER PRIMARY KEY,
    symbol      TEXT,
    direction   TEXT,           -- BUY / SELL
    entry       REAL,
    exit        REAL,
    lot         REAL,
    sl          REAL,
    tp          REAL,
    pnl         REAL,
    session     TEXT,           -- LONDON / NY / OVERLAP
    timeframe   TEXT,           -- H1 / H4 / MTF
    llm_reason  TEXT,           -- Full LLM reasoning at entry
    opened_at   DATETIME,
    closed_at   DATETIME
);

-- Post-trade lessons (written by LLM after close)
CREATE TABLE lessons (
    id          INTEGER PRIMARY KEY,
    trade_id    INTEGER REFERENCES trades(id),
    symbol      TEXT,
    session     TEXT,
    lesson      TEXT,           -- LLM's written post-mortem
    tags        TEXT,           -- JSON array: ["momentum","news_spike","fakeout"]
    weight      REAL DEFAULT 1.0, -- Adjusted by performance over time
    created_at  DATETIME
);

-- Per-pair adaptive strategy state (LRU-ranked)
CREATE TABLE pair_state (
    symbol          TEXT PRIMARY KEY,
    lru_rank        INTEGER,    -- 1 = most recently traded, higher = less active
    bias            TEXT,       -- bullish / bearish / neutral
    preferred_tf    TEXT,       -- best performing timeframe for this pair
    win_rate_7d     REAL,
    avg_pnl_7d      REAL,
    session_scores  TEXT,       -- JSON: {"LONDON":0.7,"NY":0.6,"OVERLAP":0.8}
    last_traded_at  DATETIME,
    updated_at      DATETIME
);

-- Market data cache with TTL
CREATE TABLE market_cache (
    key         TEXT PRIMARY KEY, -- e.g. "XAUUSD_H4_ohlcv"
    value       TEXT,             -- JSON payload
    source      TEXT,
    expires_at  DATETIME
);

-- Daily trade diary (append-only, one entry per trade event)
CREATE TABLE diary (
    id          INTEGER PRIMARY KEY,
    date        TEXT,           -- YYYY-MM-DD (MYT)
    entry_type  TEXT,           -- TRADE_OPEN / TRADE_CLOSE / LESSON / RESEARCH / SYSTEM
    content     TEXT,           -- Human-readable narrative
    created_at  DATETIME
);

-- Session RAM (resets at 00:00 MYT every day)
CREATE TABLE session_ram (
    key         TEXT PRIMARY KEY,
    value       TEXT,
    expires_at  DATETIME        -- always today midnight
);

-- Strategy version patches
CREATE TABLE strategy_patches (
    patch_id    TEXT PRIMARY KEY, -- e.g. "PATCH-007"
    description TEXT,
    diff        TEXT,             -- what changed in master-strategy
    applied_at  DATETIME,
    rolled_back INTEGER DEFAULT 0
);

-- Telegram conversation context
CREATE TABLE conversations (
    id          INTEGER PRIMARY KEY,
    role        TEXT,           -- user / assistant
    content     TEXT,
    created_at  DATETIME
);
```

---

### 13.3 Session RAM (Resets Every Trading Day)

Inspired by MemoryCore's `current-session.md` — temporary working memory that resets at midnight MYT.

```
session_ram contents during trading hours:
├── Current open positions snapshot
├── Today's running PnL
├── News events already flagged today
├── Signals already processed (dedup guard)
├── Active pending orders (ticket IDs, levels)
└── Cache of last MTF analysis per pair (15-min TTL)
```

At **00:00 MYT**: session RAM cleared → LEARNING mode begins with clean slate.

**Context guard**: Maximum **500 tokens of session RAM** injected per LLM call.  
If session exceeds 500 tokens → auto-summarize → replace with compressed version.

---

### 13.4 LRU Pair Ranking

Inspired by MemoryCore's LRU Project Management. Pairs are ranked by recency of trade activity:

```
pair_state.lru_rank:
  1  = XAUUSD  (traded today)
  2  = EURUSD  (traded yesterday)
  3  = USDJPY  (traded 3 days ago)
  4  = GBPUSD  (last traded 1 week ago)
```

LLM context budget allocation:
- Rank 1–2: Full lesson history injected (top 5 lessons each)
- Rank 3–4: Summary only (1-line bias + win rate)
- Unranked: Not mentioned unless explicitly asked

This prevents context bloat when watching many pairs.

---

### 13.5 Echo Memory Recall

Inspired by MemoryCore's Echo Memory Recall — keyword + tag search across trade history before each decision.

The `read_memory` skill:
```
Input: symbol="XAUUSD", context="news spike", session="LONDON"
Process:
  1. Search lessons.tags for matching keywords
  2. Weight results by lessons.weight + recency
  3. Return top-K (max 3) most relevant lessons
  4. If nothing found → returns "No prior lessons for this condition"
  5. Never fabricates — only returns what's in DB
```

Runs automatically before every LLM trade decision. Bot never enters a trade without first asking: *"Have I seen this setup before? Did it work?"*

---

### 13.6 Daily Diary & Monthly Archive

Inspired by MemoryCore's Save Diary System.

- **Every significant event** is written to `diary` table: trade opens, closes, lessons, research summaries, HALT events
- **One entry per event**, append-only, never overwritten
- **At 500+ entries in current month** → archive to `diary_archive_YYYY_MM` table
- **At end of LEARNING cycle** → LEARNING mode writes a "Day Summary" entry in natural language

```
Example diary entries (MYT timestamps):
2026-02-27 20:15 | TRADE_OPEN  | BUY XAUUSD 0.10 @ 2912.50 | Reason: H4 bullish, news clear
2026-02-27 21:40 | TRADE_CLOSE | XAUUSD closed +$47.20 | TP hit
2026-02-27 21:41 | LESSON      | XAUUSD NY session strong during USD weakness days
2026-02-27 23:55 | RESEARCH    | Tomorrow: FOMC minutes at 2:00 AM MYT — avoid EURUSD
2026-02-28 00:05 | SYSTEM      | LEARNING mode started. Nightly review in progress.
```

---

### 13.7 Strategy Versioning & Patch System

Inspired by MemoryCore's patch system. Every time LEARNING mode rewrites `master-strategy.md`, it creates a version patch:

```
PATCH-001  2026-02-27  "Reduced GBPUSD weight after 3 consecutive losses"
PATCH-002  2026-02-28  "Added XAUUSD NY overlap as preferred session"
PATCH-003  2026-03-01  "Increased confidence threshold for SUGGEST on bad news days"
```

**Rollback**: If a new patch leads to worse performance (tracked over next 20 trades), bot can roll back to the previous strategy version via `/rollback` Telegram command.

This is the **self-correcting** mechanism — not just learning from individual trades, but versioning and auditing strategy evolution over time.

---

### 13.8 Context Injection Priority (LLM Prompt Assembly)

Every LLM call assembles context in this exact order, respecting a **2000 token hard cap**:

```
1. master-strategy.md         ~300 tokens  (always included)
2. Session RAM summary         ~200 tokens  (today's context)
3. Echo recall (top-K lessons) ~300 tokens  (relevant past trades)
4. Current signal data         ~400 tokens  (OHLCV + indicators)
5. News + calendar             ~300 tokens  (market context)
6. Risk state                  ~100 tokens  (account, open positions)
7. Conversation tail           ~400 tokens  (last 5 Telegram messages)
                               ──────────
                               ~2000 tokens total cap
```

If any section would overflow → it's summarized or truncated from the bottom up (conversation tail first, then news, then echo recall).


---

## 14. Skills / Tool Dispatch

Tools the LLM can call during its ReAct loop:

| Skill | Function | Data source |
|---|---|---|
| `get_price` | Current bid/ask + spread for symbol | MT5 EA / external price API |
| `get_ohlcv` | Candle data (1m, 5m, 1h, 4h, D1) | MT5 EA |
| `analyze_technical` | RSI, MACD, EMA crossover, ATR, S/R levels | Computed in Go |
| `score_confidence` | Multi-factor confluence score (0–100) | Internal scoring engine |
| `fetch_news` | Latest forex news for symbol/currency | ForexFactory, NewsAPI, MarketAux |
| `get_sentiment` | Market sentiment score | Reddit FX feeds, social APIs |
| `get_cot_data` | COT positioning (commercial/non-commercial) | CFTC weekly report |
| `get_economic_calendar` | Upcoming high-impact events | ForexFactory calendar |
| `check_correlation` | Cross-pair correlation check | Computed in Go |
| `check_spread` | Current spread vs average — reject if too wide | MT5 EA |
| `calculate_risk` | Position size from equity + SL | Internal risk engine |
| `read_memory` | Retrieve past trade lessons (echo recall) | SQLite |
| `write_memory` | Store new lesson or insight | SQLite |
| `place_pending` | Place LIMIT/STOP pending order | REST → EA |
| `modify_pending` | Modify SL/TP on existing pending order | REST → EA |
| `cancel_pending` | Cancel pending order by ticket | REST → EA |
| `market_close` | Emergency close (HALT only) | REST → EA |
| `get_account_info` | Balance, equity, open positions | MT5 EA |

### Confidence Scoring System (`score_confidence`)

Every trade decision must pass a confidence gate before execution:

```
Confidence Score = weighted sum of:
  MTF alignment          (D1+H4+H1 agree)     0–25 pts
  S/R confluence         (key level nearby)     0–20 pts
  Session quality        (London/NY overlap)    0–15 pts
  News clearance         (no high-impact 30min) 0–10 pts
  Echo recall match      (similar setup won)    0–10 pts
  Spread check           (spread below avg)     0–10 pts
  COT alignment          (smart money agrees)   0–10 pts
                                               ________
                                               0–100 total
```

| Score | Action |
|---|---|
| 80–100 | 🟢 Execute — high conviction |
| 60–79 | 🟡 Execute with reduced lot (50%) |
| 40–59 | 🟠 Skip — log to diary as "watched but skipped" |
| 0–39 | 🔴 Hard skip — conditions too weak |

### Correlation Guard (`check_correlation`)

Prevents opening conflicting or duplicate positions:

- EURUSD long + USDCHF long = **conflicting** (negatively correlated) → blocked
- EURUSD long + GBPUSD long = **correlated** → counts as one exposure slot
- Max 2 correlated positions at any time

### Spread Filter (`check_spread`)

Rejects entries during abnormal spread conditions:

- Caches 20-period rolling average spread per pair
- If current spread > 2x average → skip and log
- Prevents entries during low liquidity or broker spread widening

---

## 15. Market Data Sources

| Category | Source | Cost | Update Frequency |
|---|---|---|---|
| Price / OHLCV | MT5 EA (primary), Twelve Data API (backup) | Free | Real-time (EA push) |
| Forex news | MarketAux API, ForexFactory RSS | Free | Every 15 min |
| Economic calendar | ForexFactory Calendar scrape | Free | Daily (Tokyo open) |
| Sentiment | Reddit r/Forex + r/algotrading RSS | Free | Hourly |
| Macro / fundamentals | Alpha Vantage (free tier) | Free | Daily |
| **COT data** | CFTC Commitments of Traders (public) | Free | Weekly (Friday release) |
| Spread data | MT5 EA (`SymbolInfoDouble`) | Free | Real-time |

All data cached in SQLite with TTL to avoid redundant calls and rate limiting.

---

## 16. Telegram Command Interface

| Command | Action |
|---|---|
| `/status` | Current mode, open positions, equity, PnL today, pending orders |
| `/mode observe\|suggest\|auto` | Switch safety mode |
| `/halt` | Emergency stop — close all, cancel pending, freeze |
| `/report` | Daily PnL summary + key insights + win rate |
| `/ask [question]` | Chat with the bot directly (LLM conversation) |
| `/learn` | Force a self-review cycle on recent trades |
| `/pairs` | Show active pairs, LRU rank, bias, confidence |
| `/pending` | List all active pending orders with levels |
| `/rollback` | Revert to previous strategy patch version |
| `/confidence` | Show current confidence score breakdown per pair |
| `/config` | Show current risk config (read-only) |

---

## 17. Go Project Structure (Proposed)

```
PhantomClaw/
├── cmd/
│   └── phantomclaw/
│       └── main.go           # Entry point, wires everything
├── internal/
│   ├── agent/                # ReAct loop, orchestrator
│   ├── bridge/               # MT5 REST HTTP server
│   ├── llm/                  # LLM provider adapters
│   │   ├── provider.go       # Interface
│   │   ├── claude.go
│   │   ├── openai.go
│   │   ├── gemini.go
│   │   └── deepseek.go
│   ├── memory/               # SQLite layer
│   ├── risk/                 # Guardrails engine
│   ├── skills/               # Tool dispatch + implementations
│   ├── telegram/             # Bot listener + commands
│   ├── market/               # Market data connectors
│   └── config/               # Config loader (config.yaml)
├── ea/
│   └── PhantomClaw.mq5       # MT5 Expert Advisor (MQL5)
├── config.yaml               # All user settings
├── go.mod
└── README.md
```

---

## 18. Additional Features (All Included)

1. ✅ **Trade journaling** — every trade reasoned in writing, stored permanently
2. ✅ **Morning brief** — 08:00 MYT Telegram digest (Tokyo data, today's setups, key events)
3. ✅ **Pre-session alert** — 14:45 MYT "London opens in 15min" with MTF bias per pair
4. ✅ **Health ping** — Telegram alert if bot hasn't reported in 6 hours (VPS crash detection)
5. ✅ **Windows service** — auto-starts on VPS reboot via Windows Service Manager
6. ✅ **Dry-run / paper mode** — full reasoning loop, no real execution, simulated PnL tracked
7. ✅ **Multi-pair watchlist** — configurable (default: EURUSD, XAUUSD, USDJPY, GBPUSD)
8. ✅ **Structured JSON logs** — daily rotating, audit-ready
9. ✅ **Session-aware mode enforcement** — outside trading hours → forced OBSERVE
10. ✅ **News spike guard** — high-impact event within 30min → pauses new entries
11. ✅ **Confidence scoring gate** — every trade scored 0–100, sub-60 = reduced lot, sub-40 = skip
12. ✅ **Correlation guard** — blocks conflicting or over-correlated positions
13. ✅ **Spread filter** — rejects entries when spread > 2x rolling average
14. ✅ **COT data integration** — weekly institutional positioning layered into bias
15. ✅ **Pending order management** — pre-places LIMIT/STOP, cancels stale orders automatically
16. ✅ **Strategy versioning** — numbered patches, `/rollback` via Telegram

---

## 19. MVP Scope (Phase 1 Deliverable)

> Build the skeleton first. Intelligence comes after the pipes work.

**MVP includes:**
- [ ] Go project structure + config system (`viper` + `config.yaml`)
- [ ] Telegram bot (listener + commands: `/status` `/halt` `/mode` `/pending` `/report`)
- [ ] LLM adapter interface + Claude implementation (others wired in Phase 2)
- [ ] MT5 REST bridge (receive signal, place/modify/cancel pending orders)
- [ ] Risk guardrail engine (hard limits, LLM cannot override)
- [ ] AUTO mode as default (pending order execution)
- [ ] SQLite memory schema (all 8 tables from §13)
- [ ] Basic skills: `get_price`, `place_pending`, `cancel_pending`, `get_account_info`
- [ ] Confidence scoring gate (basic version: MTF + S/R + session = 3 factors)
- [ ] Session scheduler (Tokyo/London/NY/LEARNING windows)
- [ ] Windows service wrapper
- [ ] MQL5 EA (`PhantomClaw.mq5`) — WebRequest + pending order management

**Phase 2 (after MVP proven stable):**
- [ ] All 4 LLM providers wired (Claude + GPT-4o + Gemini + DeepSeek)
- [ ] Full confidence scoring (7 factors with COT + echo recall)
- [ ] Correlation guard + spread filter
- [ ] Echo memory recall + daily diary system
- [ ] News/sentiment/COT data connectors
- [ ] LEARNING mode (nightly review, strategy patches, simulation runs)
- [ ] Self-correction loop + strategy versioning

**Phase 3 (long-term):**
- [ ] Backtesting module (validate S/R strategy on historical MT5 data)
- [ ] Multi-account support
- [ ] Web dashboard (optional, only if Telegram becomes limiting)
- [ ] Local LLM via Ollama (zero API cost option)

---

## 20. Open Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Broker blocks WebRequest | Localhost-only endpoint — invisible to broker |
| LLM makes bad trade | Confidence gate (sub-40 = blocked) + hard risk limits |
| Correlated overexposure | Correlation guard blocks conflicting/duplicate positions |
| Wide spread trap | Spread filter rejects entries > 2x rolling average |
| VPS crashes during open trade | Every pending order has SL/TP set by EA — positions protected |
| API rate limits | SQLite TTL cache on all external data |
| Key/secret exposure | Environment variables only, never in config.yaml |
| Runaway losses | Drawdown circuit breaker → auto-HALT + Telegram alert |
| Stale pending orders | Session scheduler cancels all unfilled orders at 00:00 MYT |
| LLM non-determinism | Confidence scoring is deterministic Go code — LLM only influences reasoning, not the final gate |
| Day-1 cold start | First 30 trades run at 50% lot size until adaptive weights initialize |

---

*PRD v1.0 — PhantomClaw — Ready for approval*
