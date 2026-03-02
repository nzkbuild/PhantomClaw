# PhantomClaw v3 — Blueprint

> Stabilize the core, harden execution paths, then unlock true agent behavior.

---

## Execution Plan

Detailed implementation plan: `docs/plans/2026-03-02-v3-foundation.md`

---

## Where We Stand Today

PhantomClaw v2 delivered major upgrades (multi-provider routing, memory, cron, tools), but production behavior still has reliability and safety gaps across bridge protocol, risk-state sync, Telegram control-plane, and data integrity.

v3 is not about adding shiny features first. It is about making the existing system deterministic, secure, and resilient under real market/runtime conditions.

---

## Data Flow: Today vs Target

```
TODAY:
  EA signal -> /signal (async hold ack) -> in-memory decision queue by symbol -> EA polls /decision
                                      \-> agent + tools + LLM
  Telegram commands -> direct handlers (no inbound ACL gate)
  Market context -> cached via stringified structs (partial/no parse back)

TARGET (v3):
  EA signal(request_id) -> durable decision pipeline (stateful queue + id correlation) -> EA ack/consume
  Telegram -> authorized control-plane + optional chat-plane (agent brain)
  Risk engine <- live equity/positions reconciliation from bridge snapshots
  Market data -> typed JSON cache + deterministic fail policy
  Ops -> health/readiness with real component truth + single-instance guardrails
```

---

## Issue Register (1–24)

| # | Issue | Severity | Why it matters | Primary Fix |
|---|-------|----------|----------------|-------------|
| 1 | Bridge contract ambiguity (sync expectation vs async design) | High | Causes operator confusion and false “timeout is broken” diagnosis | Document + enforce explicit async protocol (`accepted_async`, polling semantics) |
| 2 | Missing request correlation ID in bridge flow | Critical | Decisions can be mismatched to wrong signal cycle | Add `request_id` to `/signal` and `/decision` |
| 3 | Pending decisions keyed only by symbol | Critical | New signal can overwrite unresolved prior decision | Key pending decisions by `request_id` (and symbol index) |
| 4 | Pending decision queue is memory-only | Critical | Restart loses live decisions | Persist pending queue in SQLite with status transitions |
| 5 | `/decision` has no ack lifecycle | High | No deterministic delivery/consumption guarantees | Add `pending -> delivered -> consumed/expired` states |
| 6 | Bridge endpoints are unauthenticated (localhost trust only) | High | Local malware/process can inject signals or reads | Add shared secret/header allowlist for bridge endpoints |
| 7 | Bridge health endpoint version drift (`0.1.0`) | Low | Misleading ops/debug telemetry | Source version from runtime constant |
| 8 | Risk engine equity not updated from bridge snapshots | Critical | Drawdown checks can silently be ineffective | Wire `risk.UpdateEquity(req.Equity)` on every signal |
| 9 | Risk open-position count can drift from MT5 reality | Critical | Risk gate can approve when exposure is already high | Reconcile open positions from bridge account snapshot |
| 10 | Drawdown math uses daily loss/equity, not real peak-to-trough DD | High | Circuit-breaker logic can under/over-react | Track equity peak and compute true drawdown |
| 11 | Safety learning window hardcoded (`00:00–08:00`) | Medium | Config values are ignored | Parse and enforce configured session window |
| 12 | Session store day partition uses local clock, not bot timezone | Medium | Memory day-rollover inconsistencies near midnight | Inject timezone into `SessionStore` |
| 13 | Session alerts callback logs but does not send Telegram | High | Alerts subsystem appears active but is inert | Wire callback to `tgBot.Send` |
| 14 | Telegram inbound command ACL missing | High | Any chat/user can issue bot commands | Enforce `chat_id`/user allowlist before handling |
| 15 | Unknown Telegram text always generic; no intelligent chat path | Medium | User expects “bot brain,” gets static response | Add optional chat-mode route to agent brain |
| 16 | Telegram Markdown parse can fail on reserved chars | Medium | Replies intermittently fail or degrade | Centralized safe formatter + plain fallback |
| 17 | Agent handlers invoked with `context.Background()` | High | No cancellation/deadline control for long LLM calls | Propagate request-scoped contexts with timeout |
| 18 | News cache stores `fmt.Sprintf("%v", items)` | High | Cache cannot be reliably deserialized | Store JSON blobs with schema version |
| 19 | `parseNewsCache` / `parseCOTCache` return `nil` stubs | High | Cached reads can silently drop context | Implement robust JSON decode paths |
| 20 | News fetch errors fail-open | Medium | Risk filters weaken during data outages | Configurable fail policy (`fail_open`/`fail_closed`) |
| 21 | `cron_add` uses `time.AfterFunc` (non-durable jobs) | Medium | Scheduled wakeups disappear on restart | Persist scheduled jobs + replay on boot |
| 22 | Tool registry iteration order nondeterministic (map range) | Medium | Prompt/tool behavior varies run-to-run | Sort tool list and names deterministically |
| 23 | EA `/trade-result` payload omits `entry` while backend expects it | Medium | Post-mortem quality degraded (zero/unknown entry) | Include `entry` from history deal fields |
| 24 | EA JSON extraction is fragile string-scanning | Medium | Parser breaks on formatting/escaping changes | Implement safer parser strategy / strict response schema |

---

## Beyond 24 (v3.x Backlog)

| # | Issue | Severity | Primary Fix |
|---|-------|----------|-------------|
| 25 | Bridge component health check always reports OK | Medium | Real connectivity/readiness probes against HTTP server state |
| 26 | Recovery + rate-limiter objects are initialized but mostly not wired | Medium | Integrate into real error paths with action metrics |
| 27 | No single-instance guardrail (second process races until bind fail) | Medium | Startup lockfile/port preflight + clear operator error |
| 28 | Dynamic jobs and decision queue lack operator introspection endpoints | Low | Add admin endpoints (`/admin/jobs`, `/admin/queue`) |
| 29 | Weak integration test coverage for EA-bridge-agent end-to-end | High | Add deterministic E2E suite with mocked MT5 + Telegram |
| 30 | No migration/versioning discipline for operational data contracts | Medium | Add schema/version headers and migration checks |

---

## The 5 Pillars We Need

### Pillar 1: Deterministic Bridge Protocol
**Goal:** Every signal and decision is traceable, correlated, durable, and idempotent.

**Build:**
- `request_id` in signal and decision APIs
- Durable pending decision store (SQLite)
- Decision lifecycle states + expiration
- Bridge auth token for local endpoint protection

**Impact:** Critical

---

### Pillar 2: Risk-State Correctness
**Goal:** Risk engine reflects real MT5 account state continuously.

**Build:**
- Equity sync on every signal
- Open-position reconciliation loop
- True drawdown tracking (equity peak model)
- Hard risk gates tested against restart/manual-trade scenarios

**Impact:** Critical

---

### Pillar 3: Telegram as Reliable Control Plane
**Goal:** Telegram commands are secure, deterministic, and optionally intelligent.

**Build:**
- Inbound ACL (`chat_id`, optional `user_id`)
- Safe outbound formatter
- Real session alert delivery
- Optional chat mode (`/chat on|off`) routed to agent brain

**Impact:** High

---

### Pillar 4: Data Integrity for Market Context
**Goal:** Cached news/sentiment/COT context is typed, parseable, and policy-driven under outages.

**Build:**
- JSON cache serialization/deserialization
- Remove `nil` parse stubs
- Configurable fail policy for unavailable feeds
- Data freshness markers in prompt context

**Impact:** High

---

### Pillar 5: Operational Hardening
**Goal:** Runtime behavior is observable and recoverable under faults.

**Build:**
- Real health/readiness checks
- Single-instance startup guard
- Durable scheduler jobs
- Deterministic tool ordering
- E2E regression suite

**Impact:** High

---

## Phase Plan

### Phase A — P0 Reliability (must-fix before new features)
- [ ] Implement issues #2, #3, #4, #5 (bridge correlation + durability)
- [ ] Implement issues #8, #9, #10 (risk-state correctness)
- [ ] Implement issue #13 (real session alerts)
- [ ] Implement issue #14 (Telegram inbound ACL)
- [ ] Implement issue #17 (request-scoped context deadlines)
- [ ] Implement issue #23 (EA trade-result `entry`)

**Exit criteria:**
- No decision loss on restart
- Risk stats match MT5 snapshots during soak test
- Telegram commands accepted only from authorized chat/user

### Phase B — P1 Safety + Determinism
- [ ] Implement issues #11, #12 (timezone/session correctness)
- [ ] Implement issues #18, #19, #20 (market cache + fail policy)
- [ ] Implement issue #21 (durable cron jobs)
- [ ] Implement issue #22 (deterministic tool ordering)
- [ ] Implement issues #7, #25, #27 (ops correctness)

**Exit criteria:**
- Deterministic prompt/tool order across restarts
- Cached market feeds load reliably
- Session/day rollover is timezone-correct

### Phase C — P2 Capability Upgrade (after core is stable)
- [ ] Implement issue #15 (optional intelligent Telegram chat mode)
- [ ] Implement issue #16 (fully safe formatting layer)
- [ ] Implement issue #24 (EA parser hardening)
- [ ] Implement issues #26, #28, #29, #30 (observability + test/migration discipline)

**Exit criteria:**
- Telegram can operate as command bot and optional chat bot safely
- Bridge/EA parsing is robust under response formatting variance
- End-to-end regression suite gates releases

---

## What We Keep (Non-Negotiable Edge)

| Feature | Keep | Why |
|--------|------|-----|
| MT5 bridge specialization | Yes | Core product moat |
| Hard risk engine constraints | Yes | Safety boundary vs LLM mistakes |
| Confidence/correlation/spread guards | Yes | Trading-specific edge |
| Strategy versioning + post-mortem learning | Yes | Compounding performance over time |
| MYT session scheduling | Yes | Aligned to actual operating model |

---

## Priority Matrix

| Item | Effort | Impact | Phase |
|------|--------|--------|-------|
| Bridge correlation + durable queue | Medium | Critical | A |
| Risk sync (equity + positions + DD) | Medium | Critical | A |
| Telegram ACL + real alert send | Small | High | A |
| Request-scoped context deadlines | Small | High | A |
| Market cache integrity | Small-Medium | High | B |
| Timezone/session correctness | Small | Medium | B |
| Durable cron + deterministic tools | Small | Medium | B |
| Intelligent Telegram chat mode | Medium | Medium | C |
| Full E2E reliability suite | Medium | High | C |

---

## v3 Definition of Done

- Bridge decisions are durable, correlated, and auditable.
- Risk engine state is synchronized with MT5 at runtime.
- Telegram is secure as control-plane and reliable for alerts.
- Market context ingestion is typed and deterministic.
- Core flows are covered by end-to-end regression tests.
