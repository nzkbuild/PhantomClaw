# 🐾 PhantomClaw

> Your personal AI trading assistant that thinks, trades, and learns — all on autopilot.

PhantomClaw is an autonomous trading bot that connects to **MetaTrader 5** and uses AI to analyze markets and place trades for you. It supports **multiple LLM providers** (Claude, GPT-4o, Groq, OpenRouter, Ollama, and any OpenAI-compatible API) with automatic failover. It runs 24/7 on your Windows VPS, communicates with you through **Telegram**, and gets smarter after every trade. A built-in **Bloomberg-inspired dashboard** gives you real-time visibility into performance, risk, and system health.

**Private use only.**

---

## 📋 Table of Contents

- [What It Does](#-what-it-does)
- [What You Need](#-what-you-need)
- [Setup Guide](#-setup-guide)
- [How to Run](#-how-to-run)
- [Dashboard](#-dashboard)
- [Controlling the Bot](#-controlling-the-bot)
- [How It Works](#-how-it-works)
- [Trading Sessions](#-trading-sessions)
- [Safety Features](#-safety-features)
- [Configuration](#-configuration)
- [File Structure](#-file-structure)
- [Troubleshooting](#-troubleshooting)

---

## 🧠 What It Does

1. **Reads the market** — Your MT5 Expert Advisor sends live price data to PhantomClaw every minute
2. **Thinks with AI** — The bot asks your LLM to analyze the data, check past lessons, news, and sentiment
3. **Remembers context** — Conversation history prevents contradictory decisions within the same day
4. **Decides to trade or wait** — A confidence score (0-100) determines if the setup is strong enough
5. **Uses tools** — Can search the web for news, check prices, and schedule its own rechecks
6. **Places pending orders** — If confidence is high enough, it places a limit/stop order in MT5
7. **Learns from results** — After every trade closes, the AI writes a lesson and updates its strategy
8. **Tells you what's happening** — You get Telegram alerts for trade decisions, session changes, and health pings

---

## 📦 What You Need

Before setting up PhantomClaw, make sure you have:

| Item | Where to Get It |
|------|----------------|
| **Windows VPS** (or PC that's always on) | Any VPS provider — yours is already set up |
| **MetaTrader 5** installed on the VPS | Your broker provides this |
| **Go 1.21+** installed | [Download Go](https://go.dev/dl/) |
| **GCC compiler** (for SQLite) | Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) — pick the 64-bit version |
| **Telegram Bot token** | Talk to [@BotFather](https://t.me/BotFather) on Telegram |
| **Your Telegram Chat ID** | Talk to [@userinfobot](https://t.me/userinfobot) on Telegram |
| **AI API key** (at least one) | Get from [Anthropic](https://console.anthropic.com/), [OpenAI](https://platform.openai.com/), [Groq](https://console.groq.com/), or any OpenAI-compatible provider |

---

## 🚀 Setup Guide

### Step 1: Download the code

Open PowerShell on your VPS and run:

```powershell
cd C:\
git clone https://github.com/nzkbuild/PhantomClaw.git
cd PhantomClaw
```

### Step 2: Set your API keys

Set these environment variables in PowerShell (or place them in a local `.secrets` file). Replace the values with your actual keys:

```powershell
# Required — at least one AI provider (set in config.yaml providers list)
$env:PHANTOM_LLM_PROVIDERS_0_API_KEY = "sk-ant-your-key-here"

# Optional — add more providers for fallback
$env:PHANTOM_LLM_PROVIDERS_1_API_KEY = "sk-your-openai-key"
$env:PHANTOM_LLM_PROVIDERS_2_API_KEY = "sk-your-openrouter-key"
$env:PHANTOM_LLM_PROVIDERS_3_API_KEY = "sk-your-groq-key"
$env:PHANTOM_LLM_PROVIDERS_4_API_KEY = "sk-your-mistral-key"
$env:PHANTOM_LLM_PROVIDERS_5_API_KEY = "sk-your-deepseek-key"

# Required — Telegram
$env:PHANTOM_TELEGRAM_TOKEN = "123456:ABC-your-bot-token"
$env:PHANTOM_TELEGRAM_CHAT_ID = "your-chat-id-number"

# Optional but strongly recommended — bridge auth shared secret
$env:PHANTOM_BRIDGE_AUTH_TOKEN = "set-a-long-random-secret"

# Optional — market feed failure policy
$env:PHANTOM_MARKET_FAIL_POLICY = "fail_open"  # or fail_closed
```

> 💡 The provider order in `config.yaml` determines priority. The first provider with a valid API key becomes primary. If it fails, the bot automatically falls back to the next one.

> 💡 **To make these permanent** (survive restarts), add them to System Environment Variables:
> Settings → System → About → Advanced system settings → Environment Variables → New

### Step 3: Create local config.yaml (recommended)

```powershell
Copy-Item .\config.example.yaml .\config.yaml
```

Optional secrets file bootstrap:

```powershell
Copy-Item .\.secrets.example .\.secrets
```

The default settings are safe to start with. But if you want to adjust risk limits:

```yaml
risk:
  max_lot_size: 0.10       # Biggest trade size (start small!)
  max_daily_loss_usd: 100  # Stop trading if daily losses hit $100
  max_open_positions: 3    # Max 3 trades open at once
```

### Step 4: Build the bot

```powershell
cd C:\PhantomClaw
go build -o phantomclaw.exe ./cmd/phantomclaw/
```

If you see `CGO_ENABLED` errors, make sure TDM-GCC is installed and in your PATH.

### Step 5: Install the MT5 Expert Advisor

1. Copy `ea/PhantomClaw.mq5` to your MT5 `Experts` folder:
   ```
   C:\Users\YourName\AppData\Roaming\MetaQuotes\Terminal\YOUR_TERMINAL_ID\MQL5\Experts\
   ```
2. Open MT5 → Navigator → Expert Advisors → right-click → Refresh
3. Drag `PhantomClaw` onto your XAUUSD H1 chart
4. **Important:** Go to Tools → Options → Expert Advisors → ✅ Allow WebRequest for listed URLs → Add `http://127.0.0.1:8765`
5. If bridge auth is enabled, set the EA input `BridgeAuthToken` to the same value as `PHANTOM_BRIDGE_AUTH_TOKEN`
6. Keep EA `BridgeContractVersion` aligned with bridge contract major version (default `v3`)

### Step 6: Run it!

```powershell
cd C:\PhantomClaw
.\phantomclaw.exe
```

You should see:
```
  🐾 PhantomClaw v4.1.0
  ──────────────────────────────────────
  Config       ✓  AUTO mode · Asia/Kuala_Lumpur
  Secrets      ✓  loaded from .secrets
  Memory       ✓  SQLite ready
  Risk         ✓  max 0.10 lot · $100 daily limit
  Providers    ✓  claude (primary) + 2 fallbacks
  Bridge       ✓  127.0.0.1:8765
  Dashboard    ✓  http://127.0.0.1:8080
  Telegram     ✓  connected
  Hot Reload   ✓  watching config.yaml
  ──────────────────────────────────────
  Ready. Waiting for EA signals...
```

Open `http://127.0.0.1:8080` for the live dashboard (SSE stream + model switcher), or send `/status` on Telegram.

---

## 📊 Dashboard

PhantomClaw includes a **Bloomberg-inspired control deck** accessible at `http://127.0.0.1:8080` (configurable). The dashboard uses **Server-Sent Events (SSE)** for real-time updates — no manual refresh needed.

### Views

| View | What It Shows |
|------|---------------|
| **Control Deck** | KPI cards (mode, session, positions, daily P&L), mini equity chart, provider panel, recent decisions |
| **Equity Curve** | Full-width TradingView chart of cumulative P&L over time |
| **Decisions** | Filterable decision history with action badges and status |
| **Analytics** | Win rate, total/avg P&L, per-pair breakdown table |
| **Providers** | LLM provider status, active primary indicator, model switcher |
| **Diagnostics** | Component health, risk engine status, structured data |
| **Live Logs** | Rolling log stream with level/component/message filters |

### Configuration

```yaml
dashboard:
  host: "127.0.0.1"     # Bind address (loopback only by default)
  port: 8080            # Dashboard port
  auth_user: "admin"    # Optional basic auth username
  auth_pass: "secret"   # Optional basic auth password
```

> ⚠️ If you bind to a non-loopback address without auth configured, the bot will **automatically fall back to 127.0.0.1** for safety.

### API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/api/snapshot` | System state (mode, risk, positions) |
| GET | `/api/equity?days=30` | Cumulative P&L series for chart |
| GET | `/api/analytics?days=30` | Win rate + per-pair breakdown |
| GET | `/api/decisions?limit=200&symbol=XAUUSD` | Decision history |
| GET | `/api/sessions?limit=100&pair=XAUUSD` | Conversation/session turns |
| GET | `/api/diagnostics` | Component health (secrets auto-redacted) |
| GET | `/api/logs?level=error&limit=100` | Filtered log entries |
| GET | `/api/events` | SSE stream (snapshot + logs every 3s) |
| POST | `/api/switch-model?name=claude` | Switch primary LLM provider |

---

## 🎮 Controlling the Bot

Everything is controlled through **Telegram**. Just send these commands to your bot:

Only the configured `telegram.chat_id` is authorized to control the bot.

| Command | What It Does |
|---------|-------------|
| `/status` | Shows current mode, open positions, daily P&L, and session |
| `/halt` | 🛑 **Emergency stop** — freezes all trading immediately |
| `/mode auto` | Resume autonomous trading |
| `/mode observe` | Bot watches the market but doesn't trade |
| `/report` | Shows today's trade diary |
| `/pairs` | Shows active pairs and their win rates |
| `/confidence` | Shows the current confidence score |
| `/rollback` | Shows strategy versions (undo strategy changes) |
| `/chat on|off|status` | Toggle optional intelligent chat replies via agent brain |
| `/config` | Shows your risk settings |
| `/help` | Shows all commands |

### Most Important Commands

- **Something going wrong?** → Send `/halt` immediately
- **Want to check what it's doing?** → Send `/status`
- **Want to resume after a halt?** → Send `/mode auto`

---

## ⚙️ How It Works

```
                    You (Telegram)
                         ↕
   MT5 EA ──signals──→ PhantomClaw Bot ←──→ Dashboard (SSE)
      ↑                    │                  http://127.0.0.1:8080
      └───── decisions ────┘
                         ↕
                    AI (Claude/GPT-4o)
                         ↕
                   Memory (SQLite DB)
                   - Trade history
                   - Lessons learned
                   - Strategy document
                   - Daily diary
```

**The loop:**
1. EA pushes signal data every 60 seconds to `/signal` (fast ACK)
2. Bot processes analysis asynchronously (LLM latency does not block EA request)
3. Bot stores a correlated decision (`request_id`) in memory + SQLite (durable queue), with symbol fallback for compatibility
4. EA polls `/decision?request_id=...&symbol=...&consume=1` and executes PLACE/MODIFY/CANCEL/CLOSE actions
5. Optional explicit consume API is available: `POST /decision/consume?request_id=...`
6. Bot reconciles live account snapshot (equity + open positions) before risk checks, applies true drawdown (peak-to-current) gating, then enforces confidence/correlation/spread/risk guards
7. When a trade closes, EA posts `/trade-result` with `entry` and `exit`, and the bot writes lessons

### Bridge Protocol Notes (v3)

- `request_id`: each `/signal` request is correlated to `/decision` polling for deterministic matching.
- decision lifecycle: `pending -> delivered -> consumed|expired` (with `consume=1` or `POST /decision/consume`).
- bridge auth: if `bridge.auth_token` is set, EA must send header `X-Phantom-Bridge-Token` with matching value.
- bridge contract versioning: EA sends `X-Phantom-Bridge-Contract`; major-version mismatch is rejected with HTTP 400.
- Telegram ACL: only configured `telegram.chat_id` is authorized for inbound command handling.
- Telegram chat mode: `/chat on` routes non-command text to the agent brain, `/chat off` restores command-only behavior.
- Runtime model controls: `/model status|list|set <provider_or_alias[:model]>` and bridge `GET /models`.
- Trade-result contract: `/trade-result` requires `entry > 0` and rejects invalid payloads.
- admin introspection endpoints: `/admin/jobs` (pending cron jobs) and `/admin/queue` (active pending decisions).

### 🔧 Agent Tools

The AI agent has access to these tools:

| Tool | Purpose |
|------|---------|
| `get_price` | Current bid/ask/spread from MT5 |
| `place_pending` | Place limit/stop orders via EA bridge |
| `cancel_pending` | Cancel orders by ticket |
| `get_account_info` | Balance, equity, open positions |
| `cron_add` | Schedule future rechecks ("wake me in 30 min") |
| `web_search` | Search the web for market news |
| `web_fetch` | Read news articles and economic calendars |

---

## 🕐 Trading Sessions

PhantomClaw follows a daily schedule (all times in MYT / UTC+8):

| Time | Mode | What Happens |
|------|------|-------------|
| 00:00 – 08:00 | 🌙 LEARNING | Reviews past trades, rebuilds strategy, no trading |
| 08:00 – 15:00 | 📊 RESEARCH | Collects data, analyzes pairs, prepares orders |
| 15:00 – 00:00 | 💰 TRADING | Active trading via pending orders (London + NY sessions) |

**Key alerts you'll receive:**
- ☀️ 08:00 — Morning brief
- 📊 08:30 — Morning scan (all pairs)
- ⚡ 14:45 — "15 minutes to London open"
- 🇬🇧 15:00 — Trading mode activated
- 🔄 Every 30 min (trading) — Position check
- 💓 Every 5 min — Heartbeat health check

---

## 🛡️ Safety Features

PhantomClaw has multiple layers of protection — **the AI cannot override these:**

| Protection | What It Does |
|-----------|-------------|
| **Max daily loss** | Stops trading if losses exceed $100/day (configurable) |
| **Max lot size** | Can never place a trade larger than 0.10 lots (configurable) |
| **Max open positions** | Can never have more than 3 trades open at once |
| **Drawdown circuit breaker** | Auto-HALT if account drops 10% from peak |
| **Minimum trade interval** | Must wait 15 minutes between trades |
| **Confidence gate** | Score below 40 = trade is blocked, no exceptions |
| **Correlation guard** | Blocks conflicting trades (e.g., buy EUR/USD + sell GBP/USD) |
| **Spread filter** | Blocks trades when spread is unusually wide (>2x average) |
| **Ramp-up system** | Starts with 50% lot size until 10 profitable trades prove consistency |
| **Weekend detection** | No trading on weekends |
| **Session enforcement** | OBSERVE mode forced outside trading hours |
| **`/halt` command** | You can freeze everything instantly from Telegram |
| **Dashboard auth** | Optional Basic Auth on all dashboard endpoints |
| **Secret redaction** | API keys/tokens auto-scrubbed from `/api/diagnostics` |
| **Read-only DB** | Dashboard queries use a separate read-only SQLite connection |
| **Loopback guard** | Dashboard auto-binds to 127.0.0.1 if exposed without auth |
| **Queued model switches** | LLM provider changes are deferred while a signal is in-flight |

---

## 🔧 Configuration

Use `config.example.yaml` as the tracked template and keep your real `config.yaml` local (ignored by git).  
You can override any setting with environment variables using the `PHANTOM_` prefix.

**Examples:**
```powershell
# Override config values without editing the file
$env:PHANTOM_BOT_MODE = "OBSERVE"
$env:PHANTOM_RISK_MAX_LOT_SIZE = "0.05"
$env:PHANTOM_RISK_MAX_DAILY_LOSS_USD = "50"
$env:PHANTOM_MARKET_FAIL_POLICY = "fail_closed"
```

---

## 📁 File Structure

```
PhantomClaw/
├── phantomclaw.exe          ← The bot (after you build it)
├── config.example.yaml      ← Safe template (tracked)
├── config.yaml              ← Your local settings (gitignored)
├── VERSION                  ← Current version (4.1.0)
├── v4.1_blueprint.md        ← Hardening roadmap
├── scripts/
│   └── phantomclaw.ps1      ← Build/run/test menu script
├── ea/
│   └── PhantomClaw.mq5      ← MT5 Expert Advisor (copy to MT5)
├── data/
│   ├── phantom.db            ← Memory database (auto-created)
│   ├── sessions/             ← Conversation history (JSONL)
│   └── logs/
│       └── phantomclaw.log   ← Structured JSON log file
├── cmd/phantomclaw/
│   └── main.go               ← Entry point + wiring
├── internal/
│   ├── agent/                 ← AI decision engine (ReAct loop)
│   ├── bridge/                ← MT5 EA communication (REST API)
│   ├── config/                ← Config loading + validation
│   ├── dashboard/             ← Dashboard server + Bloomberg UI
│   │   ├── server.go          ← API routes + SSE handler
│   │   ├── security.go        ← Auth middleware + secret redaction
│   │   └── assets/index.html  ← Single-page dashboard (embedded)
│   ├── llm/                   ← AI providers + smart router
│   │   ├── generic.go         ← OpenAI-compatible adapter
│   │   ├── router.go          ← Router with cooldown + queued switches
│   │   └── errors.go          ← Error classifier
│   ├── logging/               ← Structured logging + startup banner
│   │   ├── banner.go          ← Pretty startup/shutdown output
│   │   └── query.go           ← Log file query engine
│   ├── memory/                ← Database, learning, analytics
│   │   ├── db.go              ← Trades, decisions, equity curve, pair analytics
│   │   └── session.go         ← Conversation history (JSONL)
│   ├── market/                ← Market data feeds
│   ├── risk/                  ← Safety guardrails
│   ├── safety/                ← Mode manager (AUTO/OBSERVE/HALT)
│   ├── scheduler/             ← Session cron + heartbeat
│   ├── skills/                ← Agent tools (trade, cron, web)
│   └── telegram/              ← Telegram bot commands
└── PRD.md                     ← Product Requirements Document
```

---

## ❓ Troubleshooting

### "agent brain not configured"
→ You haven't set any provider key. Set `PHANTOM_LLM_PROVIDERS_0_API_KEY` (or another configured provider index).

### MT5 EA shows "WebRequest error 4014"
→ You need to allow the URL in MT5: Tools → Options → Expert Advisors → Allow WebRequest → Add `http://127.0.0.1:8765`

### MT5 EA gets HTTP 401 from bridge
→ `BridgeAuthToken` in EA doesn't match bot `bridge.auth_token` / `PHANTOM_BRIDGE_AUTH_TOKEN`.

### Bot doesn't respond on Telegram
→ Check your `PHANTOM_TELEGRAM_TOKEN` and `PHANTOM_TELEGRAM_CHAT_ID`, send `/start`, then test `/help` and `/status`.

### "CGO_ENABLED" build error
→ Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) (64-bit). This is needed for the SQLite database.

### Bot stops when I close PowerShell
→ Run it as a background service. Quick method:
```powershell
Start-Process -WindowStyle Hidden .\phantomclaw.exe
```

### Bot says another instance is already running
→ A lock file exists at `data/phantomclaw.lock`. Stop the other process, or if it crashed, remove the stale lock file and restart.

### How do I check if it's running?
→ Send `/status` on Telegram. If you get a reply, it's alive.

### How do I stop it?
→ Send `/halt` on Telegram (freezes trading) or press `Ctrl+C` in the terminal (stops the bot).

---

## 📜 License

**Private use only.** Not for distribution.

---

*Built with Go, Claude, and a lot of coffee ☕ · v4.1.0*
