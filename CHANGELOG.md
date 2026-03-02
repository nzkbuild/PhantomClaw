# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versioned per [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
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

### Changed
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
