package alerts

import (
	"context"
	"strings"
	"sync"
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
	cfg := oa.currentConfig()

	// Degradation starts: no alert yet (debounced).
	msgs := oa.evaluate(base, opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"}, cfg)
	if len(msgs) != 0 {
		t.Fatalf("unexpected immediate alert: %v", msgs)
	}

	// Still degraded but before debounce threshold: no alert.
	msgs = oa.evaluate(base.Add(15*time.Second), opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"}, cfg)
	if len(msgs) != 0 {
		t.Fatalf("unexpected pre-debounce alert: %v", msgs)
	}

	// Past debounce threshold: one degradation alert.
	msgs = oa.evaluate(base.Add(20*time.Second), opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"}, cfg)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 degrade alert, got %d (%v)", len(msgs), msgs)
	}

	// Recovery should emit one recovery message.
	msgs = oa.evaluate(base.Add(30*time.Second), opsSnapshot{overallStatus: "GREEN", reasonCode: "ALL_HEALTHY"}, cfg)
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
	cfg := oa.currentConfig()
	redAuth := opsSnapshot{overallStatus: "RED", reasonCode: "AUTH_UNAUTHORIZED"}
	redContract := opsSnapshot{overallStatus: "RED", reasonCode: "CONTRACT_MISMATCH"}

	// Incident start + first debounced alert.
	if msgs := oa.evaluate(base, redAuth, cfg); len(msgs) != 0 {
		t.Fatalf("unexpected immediate alert: %v", msgs)
	}
	if msgs := oa.evaluate(base.Add(10*time.Second), redAuth, cfg); len(msgs) != 1 {
		t.Fatalf("expected first degrade alert, got %v", msgs)
	}

	// Reason changes but before update cooldown: no update.
	if msgs := oa.evaluate(base.Add(25*time.Second), redContract, cfg); len(msgs) != 0 {
		t.Fatalf("unexpected early update alert: %v", msgs)
	}

	// Reason changes after update cooldown: one update alert.
	if msgs := oa.evaluate(base.Add(31*time.Second), redContract, cfg); len(msgs) != 1 {
		t.Fatalf("expected update alert, got %v", msgs)
	}

	// No change and before repeat interval: no reminder.
	if msgs := oa.evaluate(base.Add(80*time.Second), redContract, cfg); len(msgs) != 0 {
		t.Fatalf("unexpected early reminder alert: %v", msgs)
	}

	// Past repeat interval: one reminder alert.
	if msgs := oa.evaluate(base.Add(92*time.Second), redContract, cfg); len(msgs) != 1 {
		t.Fatalf("expected reminder alert, got %v", msgs)
	}
}

func TestOpsAlerts_Start_ProbeSequenceFlow(t *testing.T) {
	var (
		mu   sync.Mutex
		sent []string
		idx  int
	)
	sequence := []map[string]any{
		{"overall_status": "GREEN", "overall_reason_code": "OPS_HEALTHY"},
		{"overall_status": "RED", "overall_reason_code": "AUTH_UNAUTHORIZED"},
		{"overall_status": "RED", "overall_reason_code": "AUTH_UNAUTHORIZED"},
		{"overall_status": "RED", "overall_reason_code": "AUTH_UNAUTHORIZED"},
		{"overall_status": "RED", "overall_reason_code": "CONTRACT_MISMATCH"},
		{"overall_status": "RED", "overall_reason_code": "CONTRACT_MISMATCH"},
		{"overall_status": "GREEN", "overall_reason_code": "OPS_HEALTHY"},
	}
	done := make(chan struct{})

	oa := NewOpsAlerts(OpsAlertsConfig{
		PollInterval:   20 * time.Millisecond,
		ProbeTimeout:   200 * time.Millisecond,
		DegradeFor:     35 * time.Millisecond,
		RepeatEvery:    5 * time.Minute,
		UpdateCooldown: 1 * time.Millisecond,
		Probe: func(ctx context.Context) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			if idx >= len(sequence) {
				return sequence[len(sequence)-1], nil
			}
			out := sequence[idx]
			idx++
			return out, nil
		},
		Send: func(ctx context.Context, text string) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, text)
			if len(sent) >= 3 {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		},
	})

	oa.Start()
	defer oa.Stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("timed out waiting for alerts, got %d: %v", len(sent), sent)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sent) < 3 {
		t.Fatalf("expected at least 3 alerts, got %d (%v)", len(sent), sent)
	}
	if !strings.Contains(sent[0], "Ops Degraded") {
		t.Fatalf("first alert should be degraded, got: %q", sent[0])
	}
	if !strings.Contains(sent[1], "Ops Status Update") {
		t.Fatalf("second alert should be status update, got: %q", sent[1])
	}
	if !strings.Contains(sent[2], "Ops Recovered") {
		t.Fatalf("third alert should be recovery, got: %q", sent[2])
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
