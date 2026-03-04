# PhantomClaw v4 — Blueprint

> Expand the platform: richer providers, real-time dashboard, operational maturity.
> Inspired by [OpenClaw](https://docs.openclaw.ai/) patterns, adapted for forex trading.

---

## Where We Stand Today

PhantomClaw v3 delivered a stable, deterministic foundation — durable bridge protocol, risk-state correctness, secure Telegram control plane, typed market data, and E2E test coverage. All 30 v3 issues are resolved.

v4 is about **expanding capabilities**: more LLM providers, a visual dashboard, operational tooling, and the infrastructure to run 24/5 autonomously.

---

## Gateway Architecture: Today vs Target

```
TODAY (v3):
  EA signal -> Bridge REST :8765 -> Agent (Claude or OpenAI) -> Decision queue
  Telegram -> commands only (no dashboard, no live state view)
  Config   -> edit YAML, restart to apply
  Deploy   -> bare .exe via PowerShell script

TARGET (v4):
  EA signal -> Bridge REST :8765 -> Agent (5+ providers, auto-failover) -> Decision queue
  Dashboard -> browser UI at :8080 (equity, positions, decisions, logs, sessions)
  Telegram  -> threaded messages + runtime model switching
  Config    -> hot reload via Viper WatchConfig(), secrets separated
  Deploy    -> Dockerfile + daemon/service mode, auto-restart on crash
  CI        -> GitHub Actions + secret scanning on every push
```

---

## Issue Register (1–21)

| # | Issue | Impact | Primary Fix |
|---|-------|--------|-------------|
| 1 | No visual monitoring dashboard | 🔴 Critical | Web UI with equity curve, positions, decisions, risk state, live logs |
| 2 | Only 2 LLM providers (Claude + OpenAI) | 🔴 Critical | First-class OpenRouter, Ollama, Groq, Mistral, DeepSeek support |
| 3 | Config changes require full restart | 🔴 Critical | Viper `WatchConfig()` hot reload with validation |
| 4 | No way to browse past decision chains | 🟡 High | Session/decision history browser in dashboard |
| 5 | All tools available to all providers equally | 🟡 High | Per-provider tool policy (restrict weaker models) |
| 6 | Can't list or switch models at runtime | 🟡 High | `/models` endpoint + Telegram `/model set` command |
| 7 | API keys stored in `config.yaml` in repo root | 🟡 High | Secrets management — `.secrets` file + startup warnings |
| 8 | Basic `/health` endpoint, no component-level detail | 🟡 High | Rich diagnostics: bridge, LLM, SQLite, risk state, latency |
| 9 | Zap logger with no export or live tail | 🟡 High | Log export endpoint + live tail, queryable by level/component |
| 10 | No containerization — bare `.exe` only | 🟡 High | Dockerfile + docker-compose.yml for VPS deployment |
| 11 | Foreground process via PowerShell script | 🟡 High | Windows service / systemd daemon with auto-restart |
| 12 | Basic loop detection (`maxToolRounds=5`) | 🟠 Medium | Ping-pong + no-progress detection with configurable thresholds |
| 13 | No automated security posture check | 🟠 Medium | `/security-check` command or startup audit |
| 14 | Cron jobs durable but no execution log | 🟠 Medium | Run history per cron job (timestamp, duration, result) |
| 15 | Flat Telegram messages during busy sessions | 🟠 Medium | Thread signal→analysis→decision→result flows |
| 16 | Raw trade data in SQLite, no aggregation | 🟠 Medium | Win rate, RR, Sharpe, drawdown, pair/session analytics |
| 17 | Components directly wired with callbacks | 🟠 Medium | Hooks/event system for extensible lifecycle events |
| 18 | Tests exist but no CI pipeline | 🟠 Medium | GitHub Actions + pre-commit + secret scanning |
| 19 | Viper unmarshal doesn't validate values | ⚪ Low | Config validation on load (sane ranges, valid timezone) |
| 20 | Adding tools requires recompilation | ⚪ Low | Plugin/skill hooks for runtime tool registration |
| 21 | Manual `go build` for upgrades | ⚪ Low | Auto-update mechanism with version checking |

---

## The 5 Pillars of v4

### Pillar 1: Multi-Provider Intelligence
**Goal:** Never be locked to one LLM — auto-failover across 5+ providers with cost/speed awareness.

**Build:**
- First-class OpenRouter, Ollama, Groq, Mistral, DeepSeek adapters
- Model listing endpoint + runtime switching
- Per-provider tool policy (restrict weaker models)
- Provider-specific error handling + cost tracking

**Impact:** Critical

---

### Pillar 2: Visual Dashboard
**Goal:** See everything at a glance — equity, positions, decisions, risk, logs, sessions.

**Build:**
- Lightweight web UI served from the Go binary
- Real-time WebSocket updates for live state
- Session/decision history browser
- Rich health/diagnostics panel
- Structured log viewer with live tail

**Impact:** Critical

---

### Pillar 3: Config & Secrets Hardening
**Goal:** Change settings without downtime, never leak API keys.

**Build:**
- Viper `WatchConfig()` hot reload with pre-apply validation
- Secrets separated from config (`.secrets` file or env-only)
- Config validation on load (sane ranges, valid timezone, ordered windows)
- Startup audit for security posture

**Impact:** High

---

### Pillar 4: Deployment & Operations
**Goal:** Run 24/5 autonomously on a VPS with zero babysitting.

**Build:**
- Dockerfile + docker-compose.yml
- Windows service / systemd daemon with auto-restart
- CI pipeline (GitHub Actions + secret scanning)
- Cron run history for observability

**Impact:** High

---

### Pillar 5: Agent Maturity
**Goal:** Smarter agent behavior, better observability, extensible architecture.

**Build:**
- Advanced loop detection (ping-pong, no-progress)
- Telegram message threading
- Trade performance analytics from existing data
- Hooks/event system for extensibility

**Impact:** Medium

---

## Phase Plan

### Phase A — P0 Provider Expansion + Config (Weeks 1–2)
- [ ] #3: Config hot reload via Viper `WatchConfig()`
- [ ] #7: Secrets management (`.secrets` + startup warnings)
- [ ] #19: Config validation on load
- [ ] #18: CI pipeline (GitHub Actions + secret scanning)
- [ ] #2: Expanded LLM providers (OpenRouter, Ollama, Groq, Mistral, DeepSeek)
- [ ] #6: Model listing & runtime switching
- [ ] #5: Per-provider tool policy

**Exit criteria:**
- Config changes apply without restart
- API keys never in `config.yaml`
- 5+ providers available with auto-failover
- Tests run automatically on push

### Phase B — P1 Dashboard + Observability (Weeks 3–4)
- [ ] #1: Web dashboard (equity, positions, decisions, risk state)
- [ ] #4: Session/decision history browser
- [ ] #8: Rich health/diagnostics endpoint
- [ ] #9: Structured log export + live tail

**Exit criteria:**
- Dashboard accessible at `localhost:8080`
- Full decision chain browsable per signal
- Component-level health visible
- Logs queryable by level/component/time

### Phase C — P1 Deployment Hardening (Week 5)
- [ ] #10: Dockerfile + docker-compose.yml
- [ ] #11: Daemon/service mode with auto-restart
- [ ] #16: Trade performance analytics

**Exit criteria:**
- One-command VPS deployment via Docker
- Bot survives crashes and logouts
- Win rate, Sharpe, and pair analytics computed from trade history

### Phase D — P2 Agent Maturity (Week 6+)
- [ ] #12: Better loop detection (ping-pong, no-progress)
- [ ] #13: Security audit command
- [ ] #14: Cron run history
- [ ] #17: Hooks/event system
- [ ] #15: Telegram message threading

**Backlog:**
- [ ] #20: Plugin/skill hooks
- [ ] #21: Auto-update mechanism

**Exit criteria:**
- Agent loop behavior is smarter and configurable
- Security posture auditable on demand
- Lifecycle events are extensible without core changes

---

## What We Keep (Non-Negotiable Edge)

| Feature | Keep | Why |
|---------|------|-----|
| MT5 bridge specialization | Yes | Core product moat |
| Hard risk engine constraints | Yes | Safety boundary vs LLM mistakes |
| Confidence/correlation/spread guards | Yes | Trading-specific edge |
| Strategy versioning + post-mortem learning | Yes | Compounding performance over time |
| MYT session scheduling | Yes | Aligned to actual operating model |
| Durable decision queue (v3) | Yes | No decision loss on restart |
| Telegram ACL + safe formatting (v3) | Yes | Secure control plane |

---

## Priority Matrix

| Item | Effort | Impact | Phase |
|------|--------|--------|-------|
| Config hot reload + secrets + validation | Small | Critical–High | A |
| Expanded providers + model switching + tool policy | Medium | Critical–High | A |
| CI pipeline | Small | Medium | A |
| Web dashboard + session browser | Large | Critical–High | B |
| Rich health + log export | Small | High | B |
| Dockerfile + daemon mode | Small | High | C |
| Trade analytics | Medium | Medium | C |
| Loop detection + security audit + cron history | Small | Medium | D |
| Hooks + Telegram threading | Medium | Medium | D |
| Plugin hooks + auto-update | Medium | Low | Backlog |

---

## v4 Definition of Done

- 5+ LLM providers with automatic failover and per-provider tool policy.
- Config changes apply live without restart; secrets never stored in config.
- Web dashboard provides real-time monitoring of equity, decisions, and risk.
- Bot runs as a daemon/container with auto-restart for 24/5 operation.
- CI pipeline gates every push with tests and secret scanning.
- Trade analytics computed from historical data for continuous improvement.
