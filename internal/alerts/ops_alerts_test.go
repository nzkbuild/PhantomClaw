package alerts

import (
	"testing"
	"time"
)

func TestOpsAlerts_DebounceThenAlertAndRecover(t *testing.T) {
	oa := NewOpsAlerts(OpsAlertsConfig{
		DegradeFor:     20 * time.Second,
		RepeatEvery:    10 * time.Minute,
		UpdateCooldown: 60 * time.Second,
	})

	base := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	// Degradation starts: no alert yet (debounced).
	msgs := oa.evaluate(base, opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"})
	if len(msgs) != 0 {
		t.Fatalf("unexpected immediate alert: %v", msgs)
	}

	// Still degraded but before debounce threshold: no alert.
	msgs = oa.evaluate(base.Add(15*time.Second), opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"})
	if len(msgs) != 0 {
		t.Fatalf("unexpected pre-debounce alert: %v", msgs)
	}

	// Past debounce threshold: one degradation alert.
	msgs = oa.evaluate(base.Add(20*time.Second), opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 degrade alert, got %d (%v)", len(msgs), msgs)
	}

	// Recovery should emit one recovery message.
	msgs = oa.evaluate(base.Add(30*time.Second), opsSnapshot{overallStatus: "GREEN", reasonCode: "ALL_HEALTHY"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 recovery alert, got %d (%v)", len(msgs), msgs)
	}
}

func TestOpsAlerts_UpdateAndReminderCooldowns(t *testing.T) {
	oa := NewOpsAlerts(OpsAlertsConfig{
		DegradeFor:     10 * time.Second,
		RepeatEvery:    60 * time.Second,
		UpdateCooldown: 20 * time.Second,
	})

	base := time.Date(2026, 3, 4, 9, 10, 0, 0, time.UTC)
	redAuth := opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"}
	redContract := opsSnapshot{overallStatus: "RED", reasonCode: "CONTRACT_MISMATCH"}

	// Incident start + first debounced alert.
	if msgs := oa.evaluate(base, redAuth); len(msgs) != 0 {
		t.Fatalf("unexpected immediate alert: %v", msgs)
	}
	if msgs := oa.evaluate(base.Add(10*time.Second), redAuth); len(msgs) != 1 {
		t.Fatalf("expected first degrade alert, got %v", msgs)
	}

	// Reason changes but before update cooldown: no update.
	if msgs := oa.evaluate(base.Add(25*time.Second), redContract); len(msgs) != 0 {
		t.Fatalf("unexpected early update alert: %v", msgs)
	}

	// Reason changes after update cooldown: one update alert.
	if msgs := oa.evaluate(base.Add(31*time.Second), redContract); len(msgs) != 1 {
		t.Fatalf("expected update alert, got %v", msgs)
	}

	// No change and before repeat interval: no reminder.
	if msgs := oa.evaluate(base.Add(80*time.Second), redContract); len(msgs) != 0 {
		t.Fatalf("unexpected early reminder alert: %v", msgs)
	}

	// Past repeat interval: one reminder alert.
	if msgs := oa.evaluate(base.Add(92*time.Second), redContract); len(msgs) != 1 {
		t.Fatalf("expected reminder alert, got %v", msgs)
	}
}

func TestParseOpsSnapshot_FallbackToNestedOverall(t *testing.T) {
	payload := map[string]any{
		"overall": map[string]any{
			"status":      "amber",
			"reason_code": "QUEUE_STUCK",
		},
		"queue_depth_active":    float64(12),
		"last_signal_age_sec":   float64(45),
		"auth_failures_5m":      float64(3),
		"contract_mismatch_5m":  float64(1),
		"decision_ready_p95_ms": float64(4200),
	}

	snap, err := parseOpsSnapshot(payload)
	if err != nil {
		t.Fatalf("parseOpsSnapshot: %v", err)
	}
	if snap.overallStatus != "AMBER" {
		t.Fatalf("overallStatus=%q, want=AMBER", snap.overallStatus)
	}
	if snap.reasonCode != "QUEUE_STUCK" {
		t.Fatalf("reasonCode=%q, want=QUEUE_STUCK", snap.reasonCode)
	}
	if snap.queueDepth != 12 || snap.lastSignalAgeSec != 45 {
		t.Fatalf("unexpected metrics: %+v", snap)
	}
}
