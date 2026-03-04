# Phase B Dashboard + Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver v4 Phase B with a live dashboard (default `:8080`, configurable host/port with fallback), decision/session history browsing, rich diagnostics, and structured log export/tail.

**Architecture:** Keep bridge API (`:8765`) as system-of-record while adding a dedicated dashboard server served by the Go binary. Expose lightweight JSON endpoints for snapshot/history/logs and render a single embedded HTML app that polls these APIs. Extend existing bridge/admin capabilities with pluggable diagnostics callbacks to avoid tight coupling.

**Tech Stack:** Go stdlib HTTP + `embed`, existing SQLite memory layer, existing zap JSON logs, minimal vanilla JS dashboard.

---

### Task 1: Add memory queries for dashboard history

**Files:**
- Modify: `internal/memory/db.go`
- Test: `internal/memory/db_test.go`

**Steps:**
1. Add `DecisionHistoryRecord` and `ListDecisionHistory(limit, symbol)` query helpers over `pending_decisions`.
2. Add `TradeSummary` and `GetTradeSummary(days)` aggregates (win rate, avg pnl, total pnl, max drawdown proxy, counts).
3. Write DB tests for both methods.

### Task 2: Add bridge diagnostics + logs endpoints

**Files:**
- Modify: `internal/bridge/server.go`
- Test: `internal/bridge/server_test.go`

**Steps:**
1. Add callback hooks to server for diagnostics and log provider.
2. Add endpoints:
   - `GET /health/diagnostics`
   - `GET /admin/decisions?limit=&symbol=`
   - `GET /admin/logs?level=&component=&contains=&limit=`
3. Add tests for payload shape and auth behavior.

### Task 3: Add dashboard server package

**Files:**
- Create: `internal/dashboard/server.go`
- Create: `internal/dashboard/assets/index.html`
- Create: `internal/dashboard/server_test.go`

**Steps:**
1. Create standalone dashboard HTTP server with config-driven host/port (default `127.0.0.1:8080`) and safe port fallback behavior.
2. Serve embedded `index.html` plus `/api/*` passthrough endpoints to local providers.
3. Implement periodic polling in UI for live metrics, decisions, sessions, diagnostics, and logs.
4. Add server tests for root page and one API endpoint.

### Task 4: Wire dashboard + rich diagnostics in main

**Files:**
- Modify: `cmd/phantomclaw/main.go`
- Modify: `README.md`

**Steps:**
1. Register bridge diagnostics/log callbacks using health monitor, DB summary, router status, bridge probe, and log reader.
2. Start dashboard server in main lifecycle and include startup logging.
3. Document dashboard URL and new APIs.

### Task 5: Verify and stabilize

**Files:**
- Modify as needed from failing tests.

**Steps:**
1. Run `gofmt` on changed Go files.
2. Run `go test ./...`.
3. Run `go build ./cmd/phantomclaw`.
4. Fix failures until green.
