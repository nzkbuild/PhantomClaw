# 🔍 V2 Audit — Need to Fix

## 🔴 Bugs (Fix Immediately)

### Bug 1: Data Race in Router
**File:** `internal/llm/router.go:251-259`

`isCoolingDownLocked()` calls `delete(r.cooldowns, provider)` (write) but is called from `ProviderStatus()` under `RLock` (read lock). Data race → crash under Go race detector.

**Fix:** Remove the `delete` from `isCoolingDownLocked`. Let `recordSuccess` clean it up.

---

### Bug 2: `cron_add` Repeats Forever
**File:** `internal/skills/cron.go:85`

Uses `@every 30m` which fires **every 30 minutes forever**, not once. Agent says "wake me in 30 min" → fires infinitely.

**Fix:** Use `time.AfterFunc()` for one-shot, or save `cron.EntryID` and call `RemoveDynamic(id)` after first fire.

---

### Bug 3: Tool Role Mapped Wrong
**File:** `internal/llm/generic.go:204-206`

`role:"tool"` → `"assistant"` breaks OpenAI tool calling flow. Model needs `role: "tool"` to see tool results correctly.

**Fix:** Remove the role mapping. OpenAI API supports `tool` role natively.

---

## 🟡 Should Fix

### Misconfig 1: HeartbeatConfig No Defaults
`config.go` — `IntervalMin` defaults to `0` if missing from yaml. The constructor handles it, but config should have explicit defaults.

### Misconfig 2: `sessions_dir` Fallback in Wrong Place
`main.go` — fallback `data/sessions` hardcoded in main.go instead of config defaults function.

### Incomplete: Stub Tools
`mvp.go` — `get_price` and `get_account_info` return hardcoded zeros. Need wiring to MT5 bridge.

---

## ⚪ Minor / Deferred

- `web_search` DDG HTML parsing is fragile (regex on class names)
- `StreamChat` is fake-streaming (delivers all at once)

---

## Priority

| # | Issue | Effort | Status |
|---|-------|--------|--------|
| 1 | Router data race | 5 min | [x] |
| 2 | cron_add repeats | 10 min | [x] |
| 3 | Tool role mapping | 5 min | [x] |
| 4 | Heartbeat defaults | 3 min | [x] |
| 5 | sessions_dir default | 3 min | [x] |
| 6 | Stub tools | 30 min | [x] |
| 7 | DDG parsing | 20 min | [x] |
| 8 | Fake streaming | 1 hour | [x] |
