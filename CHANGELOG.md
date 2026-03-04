# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versioned per [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Bridge operational truth endpoint `GET /health/ops` with canonical sectioned status (`overall`, `ea_link`, `bridge_auth`, `contract_compat`, `decision_loop`, `ai_health`, `dashboard_sync`, `data_freshness`)
- Bridge telemetry for ops health: auth failure counters, contract mismatch counters, signal ACK latency, decision-ready latency, decision-consume latency, queue age/depth metrics
- Flat ops keys in `/health/ops` payload for EA-friendly parsing (`overall_status`, `overall_reason_code`, `*_status`, age and queue metrics)
- Dashboard API endpoint `GET /api/ops`
- Dashboard SSE `ops` event on `/api/events` stream
- Dashboard Operations Truth panel + topbar ops pill + freshness labels for diagnostics/logs/decisions
- Telegram `/status` extension with ops summary block (overall status, reason code, EA signal age, queue depth, auth failures)
- EA on-chart status panel with periodic `/health/ops` polling and HTTP error classification
- Bridge tests for ops endpoint behavior and v4.2 degradation reason-code paths (`AUTH_UNAUTHORIZED`, `CONTRACT_MISMATCH`, `QUEUE_STUCK`)
- Bridge request correlation support via `request_id` on `/signal` and `/decision`
- Bridge tests for correlated decision fetch and backward-compatible symbol polling
- Durable `pending_decisions` SQLite table and DB APIs for bridge decision persistence
- Bridge tests for DB-backed decision delivery across in-memory queue loss (restart simulation)
- `POST /decision/consume` endpoint for explicit decision consumption by `request_id`
- Risk engine reconciliation API `SyncAccountSnapshot(equity, open_positions)`
- Risk tests covering snapshot-driven open-position reconciliation and drawdown gating
- Telegram ACL unit tests for configured/mismatched chat and nil payload handling
- `makeSessionAlertSender` helper + unit tests for session alert dispatch
- Bridge signal context timeout test (`TestSignalContextTimeout`)
- Trade-result contract tests for mandatory `entry` (`TestTradeResultIncludesEntry`, `TestTradeResultRejectsMissingEntry`)
- Optional bridge endpoint auth token (`bridge.auth_token`) enforced via `X-Phantom-Bridge-Token`
- Bridge auth tests for unauthorized rejection and authorized request flow
- `market.fail_policy` config (`fail_open` / `fail_closed`) for news-gate behavior during fetch/parse outages
- Unit tests for configured learning windows, timezone-aware session files, and market cache/fail-policy behavior
- Durable `cron_jobs` store and cron replay on startup for `cron_add` one-shot jobs
- Unit tests for durable cron replay, deterministic registry ordering, bridge health version output, and startup lock behavior
- Single-instance startup lockfile guard (`phantomclaw.lock`)
- Telegram chat mode toggle command (`/chat on|off|status`) with optional agent-brain responder
- Bridge admin introspection endpoints: `GET /admin/jobs` and `GET /admin/queue`
- Bridge contract version header support (`X-Phantom-Bridge-Contract`) with major-version compatibility checks
- SQLite schema metadata table (`metadata`) with startup schema-version guard
- Deterministic bridge E2E regression test covering mocked MT5 signal/decision/trade-result plus mocked Telegram chat path
- Dashboard endpoint tests for `/api/equity` and `/api/analytics` query handling
- Memory tests ensuring trade summary, equity curve, and pair analytics exclude open trades

### Changed
- Dashboard route surface increased to include `/api/ops` (11 dashboard routes total)
- README synced with operational-truth flow and ops endpoints (`/health/ops`, `/api/ops`, SSE `ops`)
- EA now attaches `request_id` in signal payloads and includes it when polling `/decision`
- Bridge now generates a request ID when absent to preserve compatibility with older EA behavior
- Bridge `NewServer` now accepts memory DB handle and persists pending decisions with TTL
- Bridge decision lifecycle now follows `pending -> delivered -> consumed|expired`
- EA decision polling now sends `consume=1` to preserve one-shot execution behavior
- Signal callback now reconciles risk engine snapshot before evaluating each signal
- Drawdown circuit breaker now uses true peak-to-current equity drawdown
- Telegram command handlers now enforce inbound `chat_id` authorization
- Session alerts now send through Telegram bot callback instead of log-only side effects
- Bridge signal callbacks now run with bounded request-scoped context and propagate `ctx` into `brain.HandleSignal`
- Bridge `/trade-result` now validates that `entry` is present and `> 0` before processing
- EA trade-result payload now includes resolved weighted entry price from position history
- Documentation synced for v3 Phase A completion status and bridge protocol contract details
- EA now supports `BridgeAuthToken` input and sends it as `X-Phantom-Bridge-Token` when configured
- Safety learning-hours enforcement now uses configured session window values (supports wrap-around windows)
- Session JSONL day partitioning now uses configured bot timezone instead of host-local timezone
- News and COT market caches now persist typed JSON payloads and decode from JSON on cache reads
- Skill registry `List()` and `Names()` now return deterministic sorted order
- Bridge `/health` version now reflects runtime app version instead of a hardcoded legacy value
- Health monitor bridge component now performs real `/health` probe instead of constant-OK stub
- Telegram send path now retries with escaped MarkdownV2 before plain-text fallback
- EA response parsing now validates required fields per action and uses safer JSON string/number extraction
- Market connectors (news/sentiment/COT) now wire rate limiter + recovery hooks into live fetch/cache paths
- Bridge callbacks in main runtime now record recovery events and enforce LLM rate-limit guard in the signal path
- EA now sends `X-Phantom-Bridge-Contract` header (configurable via `BridgeContractVersion`, default `v3`)
- Dashboard snapshot risk thresholds now come from live runtime risk config (hot-reload consistent)
- Dashboard bind hardening: non-loopback host without auth now forces loopback bind
- Equity/analytics queries now operate on closed trades only (`closed_at IS NOT NULL`)
- Router signal-switch guard now uses an atomic critical section to avoid mid-signal switch races

## [2.0.0] - 2026-03-02

### Added
- **Multi-provider LLM system** — config-driven providers list (Claude, OpenAI, Groq, OpenRouter, Ollama, any OpenAI-compatible)
- **Smart router** with error-aware fallback, per-provider cooldown, and model aliases
- **Error classifier** — categorizes provider failures (rate_limit, auth, model_not_found, network, overloaded)
- **Generic OpenAI-compatible adapter** (`generic.go`) — one adapter for any `/v1/chat/completions` endpoint
- **JSONL session store** — per-day conversation history, agent remembers previous decisions
- **Conversation history injection** — last 5 turns per pair in system prompt
- **Heartbeat** — periodic health check with alerts
- **Morning scan** (08:30 MYT) and position check (every 30 min during trading)
- **Dynamic cron** — `AddDynamic()` method for runtime job scheduling
- **`cron_add` tool** — agent can schedule its own future rechecks
- **`web_search` tool** — DuckDuckGo web search (no API key required)
- **`web_fetch` tool** — fetch and extract text from web pages
- **Loop detection** — breaks ReAct loop if same tool+args called 3x
- **Tool documentation** — human-readable tool descriptions in system prompt

### Changed
- `maxToolRounds` increased from 3 to 5 (with loop detection as safety net)
- LLM config uses providers list (`config.yaml`) instead of per-provider sections
- Environment variables now follow `PHANTOM_LLM_PROVIDERS_N_*` format
- Agent brain logs "with conversation memory" on init

### Removed
- Hardcoded `deepseek.go` — replaced by generic adapter

## [0.1.0] - 2026-02-27

### Added
- Initial project scaffold
- `README.md` with project overview
- `.gitignore` with standard exclusions
- `CHANGELOG.md` for tracking version history
- `VERSION` file for programmatic version access
