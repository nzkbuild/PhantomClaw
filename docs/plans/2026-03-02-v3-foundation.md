# V3 Foundation Reliability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver PhantomClaw v3 Phase A reliability baseline (durable/correlated bridge, risk-state correctness, secure Telegram control-plane, and accurate trade-result payloads).

**Architecture:** Introduce a request-correlated bridge contract with persistent decision state, wire MT5 account snapshots into risk state reconciliation, enforce Telegram inbound authorization, and close protocol/data mismatches in EA trade-result reporting. Implement in small, test-first increments with strict regression checks after each task.

**Tech Stack:** Go 1.24, net/http, SQLite (existing memory DB), MT5 MQL5 EA, Telegram Bot API integration.

---

## Progress Log

- 2026-03-02: Task 1 completed (bridge `request_id` correlation + EA request_id polling + bridge tests).
- 2026-03-02: Task 2 completed (durable `pending_decisions` SQLite queue + bridge DB fallback + lifecycle tests).
- 2026-03-02: Task 3 completed (pending→delivered→consumed decision lifecycle + explicit consume semantics + consume endpoint).
- 2026-03-02: Task 4 completed (risk snapshot reconciliation for equity + open positions, wired before signal risk checks).
- 2026-03-02: Task 5 completed (true drawdown gate uses peak-to-current equity, not dailyLoss/equity proxy).

---

## Scope and Sequence

1. Bridge protocol and persistence (#2, #3, #4, #5)
2. Risk-state correctness (#8, #9, #10)
3. Telegram control-plane reliability/security (#13, #14)
4. Request context hardening (#17)
5. EA trade-result contract fix (#23)

---

### Task 1: Add Bridge Contract Types for Correlation

**Files:**
- Modify: `internal/bridge/server.go`
- Modify: `ea/PhantomClaw.mq5`
- Test: `internal/bridge/server_test.go`

**Step 1: Write failing bridge contract test**

Create test asserting:
- `/signal` accepts and returns `request_id`
- `/decision` can query by `request_id` (primary) with symbol compatibility fallback

**Step 2: Run test to verify it fails**

Run: `go test ./internal/bridge -run TestBridgeRequestCorrelation -v`
Expected: FAIL (missing fields/handlers).

**Step 3: Add request correlation fields**

Implement:
- `SignalRequest.RequestID string`
- `SignalResponse.RequestID string`
- parse `request_id` on `/signal`, include in ACK
- support `/decision?request_id=...` query path

**Step 4: Run test to verify it passes**

Run: `go test ./internal/bridge -run TestBridgeRequestCorrelation -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/bridge/server.go internal/bridge/server_test.go ea/PhantomClaw.mq5
git commit -m "feat(bridge): add request correlation id to signal/decision contract"
```

---

### Task 2: Persist Pending Decisions in SQLite

**Files:**
- Modify: `internal/memory/schema.go`
- Modify: `internal/memory/db.go`
- Modify: `internal/bridge/server.go`
- Test: `internal/memory/db_test.go`
- Test: `internal/bridge/server_test.go`

**Step 1: Write failing persistence tests**

Add tests for:
- insert/get pending decision by `request_id`
- lifecycle updates (`pending`, `delivered`, `consumed`, `expired`)

**Step 2: Run tests to verify failures**

Run:
- `go test ./internal/memory -run TestPendingDecisionLifecycle -v`
- `go test ./internal/bridge -run TestDecisionPersistence -v`
Expected: FAIL (table/methods missing).

**Step 3: Implement schema + DB methods**

Add `pending_decisions` table and DB helpers:
- `InsertPendingDecision`
- `GetPendingDecision`
- `UpdatePendingDecisionStatus`
- `ExpireOldPendingDecisions`

**Step 4: Wire bridge to durable store**

Replace in-memory-only pending map usage with DB-backed decision lifecycle.

**Step 5: Run tests**

Run: `go test ./internal/memory ./internal/bridge -v`
Expected: PASS.

**Step 6: Commit**

```bash
git add internal/memory/schema.go internal/memory/db.go internal/memory/db_test.go internal/bridge/server.go internal/bridge/server_test.go
git commit -m "feat(bridge): persist pending decisions with lifecycle states"
```

---

### Task 3: Enforce Decision Delivery Semantics

**Files:**
- Modify: `internal/bridge/server.go`
- Test: `internal/bridge/server_test.go`

**Step 1: Write failing test for delivery/consume semantics**

Test behavior:
- first read transitions to `delivered`
- explicit consume endpoint or consume flag marks `consumed`
- subsequent reads return no pending decision

**Step 2: Run failing test**

Run: `go test ./internal/bridge -run TestDecisionDeliveryLifecycle -v`
Expected: FAIL.

**Step 3: Implement state transitions**

Implement deterministic transitions and response metadata (`status`, timestamps).

**Step 4: Run test**

Run: `go test ./internal/bridge -run TestDecisionDeliveryLifecycle -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/bridge/server.go internal/bridge/server_test.go
git commit -m "feat(bridge): add deterministic decision delivery and consume semantics"
```

---

### Task 4: Wire Equity and Position Reconciliation into Risk Engine

**Files:**
- Modify: `internal/risk/engine.go`
- Modify: `internal/agent/agent.go`
- Modify: `cmd/phantomclaw/main.go`
- Test: `internal/risk/engine_test.go`

**Step 1: Write failing risk sync tests**

Test:
- `UpdateEquity` is reflected in drawdown checks
- open positions can be reconciled from snapshot

**Step 2: Run failing tests**

Run: `go test ./internal/risk -run TestRiskSnapshotReconciliation -v`
Expected: FAIL.

**Step 3: Implement reconciliation methods**

Add methods for snapshot sync:
- `SyncAccountSnapshot(equity float64, openPositions int)`
- optional equity-peak tracking primitive for future DD upgrade

Wire from signal handling path before `CheckTrade`.

**Step 4: Run tests**

Run: `go test ./internal/risk -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/risk/engine.go internal/risk/engine_test.go internal/agent/agent.go cmd/phantomclaw/main.go
git commit -m "fix(risk): reconcile equity and open positions from live bridge snapshots"
```

---

### Task 5: Implement True Drawdown Tracking

**Files:**
- Modify: `internal/risk/engine.go`
- Test: `internal/risk/engine_test.go`

**Step 1: Write failing true-DD test**

Test peak-to-current drawdown behavior with synthetic equity path.

**Step 2: Run failing test**

Run: `go test ./internal/risk -run TestTrueDrawdownCircuitBreaker -v`
Expected: FAIL.

**Step 3: Implement true drawdown logic**

Track:
- `equityPeak`
- current drawdown %
- gate using configured max drawdown

**Step 4: Run tests**

Run: `go test ./internal/risk -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/risk/engine.go internal/risk/engine_test.go
git commit -m "fix(risk): enforce true peak-to-trough drawdown circuit breaker"
```

---

### Task 6: Enforce Telegram Inbound Authorization

**Files:**
- Modify: `internal/telegram/bot.go`
- Modify: `cmd/phantomclaw/main.go`
- Test: `internal/telegram/bot_test.go`

**Step 1: Write failing ACL tests**

Test:
- configured `chat_id` accepted
- non-configured `chat_id` rejected/logged

**Step 2: Run failing test**

Run: `go test ./internal/telegram -run TestInboundAuthorization -v`
Expected: FAIL.

**Step 3: Implement ACL check**

Add centralized guard at handler entry before command dispatch.

**Step 4: Run tests**

Run: `go test ./internal/telegram -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/telegram/bot.go internal/telegram/bot_test.go cmd/phantomclaw/main.go
git commit -m "fix(telegram): enforce inbound chat authorization for command handlers"
```

---

### Task 7: Wire Session Alerts to Real Telegram Send Path

**Files:**
- Modify: `cmd/phantomclaw/main.go`
- Test: `cmd/phantomclaw/main_test.go` (or targeted integration test under `internal/alerts`)

**Step 1: Write failing alert dispatch test**

Test callback invokes send function instead of log-only side effect.

**Step 2: Run failing test**

Run: `go test ./cmd/phantomclaw -run TestSessionAlertsSend -v`
Expected: FAIL.

**Step 3: Implement actual send callback**

Replace logger-only callback with `tgBot.Send(ctx, text)`.

**Step 4: Run test**

Run: `go test ./cmd/phantomclaw -run TestSessionAlertsSend -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/phantomclaw/main.go cmd/phantomclaw/main_test.go
git commit -m "fix(alerts): wire session alert callback to telegram sender"
```

---

### Task 8: Propagate Request-Scoped Context Deadlines

**Files:**
- Modify: `cmd/phantomclaw/main.go`
- Modify: `internal/bridge/server.go`
- Test: `internal/bridge/server_test.go`

**Step 1: Write failing context cancellation test**

Test long-running signal processing honors context timeout/cancellation.

**Step 2: Run failing test**

Run: `go test ./internal/bridge -run TestSignalContextTimeout -v`
Expected: FAIL.

**Step 3: Implement context propagation**

Pass request-scoped context with bounded timeout from bridge callback to agent handlers.

**Step 4: Run tests**

Run: `go test ./internal/bridge ./cmd/phantomclaw -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/bridge/server.go internal/bridge/server_test.go cmd/phantomclaw/main.go
git commit -m "fix(runtime): propagate request-scoped contexts to signal and trade handlers"
```

---

### Task 9: Fix EA Trade-Result Payload Contract (`entry`)

**Files:**
- Modify: `ea/PhantomClaw.mq5`
- Modify: `internal/bridge/server.go` (if additional validation needed)
- Test: `internal/bridge/server_test.go`

**Step 1: Write failing payload decode test**

Assert trade-result payload includes `entry` and decodes non-zero when available.

**Step 2: Run failing test**

Run: `go test ./internal/bridge -run TestTradeResultIncludesEntry -v`
Expected: FAIL.

**Step 3: Implement EA payload update**

Populate `entry` in JSON from historical deal/order fields.

**Step 4: Run tests**

Run: `go test ./internal/bridge -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add ea/PhantomClaw.mq5 internal/bridge/server.go internal/bridge/server_test.go
git commit -m "fix(ea): include entry price in trade-result payload for accurate post-mortem"
```

---

### Task 10: Docs Sync and Release Notes

**Files:**
- Modify: `v3_blueprint.md`
- Modify: `CHANGELOG.md`
- Modify: `README.md` (bridge protocol and Telegram ACL notes)

**Step 1: Update docs**

Add:
- v3 Phase A completed checklist
- new bridge protocol examples
- Telegram authorization behavior

**Step 2: Validate docs consistency**

Run:
- `rg -n "request_id|decision lifecycle|Telegram ACL|entry" README.md CHANGELOG.md v3_blueprint.md`
Expected: all topics present.

**Step 3: Commit**

```bash
git add v3_blueprint.md CHANGELOG.md README.md
git commit -m "docs(v3): sync protocol, risk, and telegram control-plane changes"
```

---

## Verification Gates (Run After Every Task)

Run:
- `go test ./...`
- `go build -o phantomclaw.exe ./cmd/phantomclaw/`

Expected:
- all tests pass
- binary builds without errors

If any fail:
- stop, fix immediately, do not continue to next task.

---

## Rollout Strategy

1. Deploy Go service updates first (bridge backward-compatible mode for symbol polling).
2. Deploy EA update with `request_id` + `entry`.
3. Enable strict request_id-only decision mode after EA confirmed upgraded.
4. Monitor logs for:
   - decision lifecycle transitions
   - unauthorized Telegram inbound attempts
   - risk reconciliation deltas

---

## Start Here (Recommended First Action)

Start with **Task 1** (request correlation) immediately, because Tasks 2–3 depend on its contract.

After Task 1, execute Task 2 (durable queue) before any Telegram or risk work to prevent decision-loss regressions during ongoing development.

---

Plan complete and saved to `docs/plans/2026-03-02-v3-foundation.md`. Two execution options:

1. Subagent-Driven (this session) - I dispatch fresh subagent per task, review between tasks, fast iteration
2. Parallel Session (separate) - Open new session with executing-plans, batch execution with checkpoints

Which approach?
