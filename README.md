# PhantomClaw 🐾

**Autonomous AI Trading Agent** — Go + Claude + MT5 + Telegram

PhantomClaw is a personal trading agent that thinks with LLMs, executes via MT5 pending orders, learns from every trade, and communicates through Telegram. It runs 24/7 on a Windows VPS.

## Quick Start

```bash
# 1. Clone
git clone https://github.com/nzkbuild/PhantomClaw.git
cd PhantomClaw

# 2. Configure
cp config.yaml config.local.yaml
# Edit config.local.yaml — set API keys via env vars:
#   PHANTOM_TELEGRAM_TOKEN=your_token
#   PHANTOM_LLM_CLAUDE_API_KEY=your_key

# 3. Build
go build -o phantomclaw.exe ./cmd/phantomclaw/

# 4. Run
.\phantomclaw.exe -config config.yaml
```

## Architecture

```
YOU (Telegram) ←→ Go Agent ←→ MT5 EA (pending orders)
                    ↕
              SQLite Memory
              LLM (Claude)
```

## Telegram Commands

| Command | Action |
|---------|--------|
| `/status` | Mode, positions, PnL, session |
| `/mode auto\|suggest\|observe\|halt` | Switch mode |
| `/halt` | Emergency stop |
| `/report` | Daily summary |
| `/pairs` | Active pairs |
| `/pending` | Pending orders |
| `/confidence` | Confidence scores |
| `/config` | Risk config |

## Project Structure

```
PhantomClaw/
├── cmd/phantomclaw/main.go     # Entry point
├── internal/
│   ├── bridge/                 # MT5 REST bridge
│   ├── config/                 # Config loader (Viper)
│   ├── llm/                    # LLM provider adapters
│   ├── memory/                 # SQLite layer
│   ├── risk/                   # Risk guardrails
│   ├── safety/                 # Mode management
│   ├── scheduler/              # Session scheduler
│   ├── skills/                 # Tool dispatch + confidence scoring
│   └── telegram/               # Bot commands
├── ea/PhantomClaw.mq5          # MT5 Expert Advisor
├── config.yaml                 # Default config
└── PRD.md                      # Product Requirements Document
```

## Trading Sessions (MYT/UTC+8)

| Time | Mode | Activity |
|------|------|----------|
| 00:00–08:00 | LEARNING | Review, strategy patches, DB housekeeping |
| 08:00–15:00 | RESEARCH | Data collection, MTF analysis, pending order placement |
| 15:00–00:00 | TRADING | Live execution via pending orders |

## Safety

- **AUTO mode** (default) — trades within hard risk limits
- Risk guardrails in Go code — LLM cannot override
- Drawdown circuit breaker → auto-HALT
- Confidence scoring gate (score < 40 = blocked)
- `/halt` emergency stop via Telegram

## License

Private — Personal use only
