package alerts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// OpsProbe fetches operational truth from the bridge (/health/ops).
type OpsProbe func(ctx context.Context) (map[string]any, error)

// OpsAlertsConfig configures ops degradation/recovery alerting.
type OpsAlertsConfig struct {
	PollInterval   time.Duration
	ProbeTimeout   time.Duration
	DegradeFor     time.Duration
	RepeatEvery    time.Duration
	UpdateCooldown time.Duration
	Probe          OpsProbe
	Send           TelegramSender
}

// OpsAlerts emits Telegram alerts on sustained ops degradation and recovery.
type OpsAlerts struct {
	cfg OpsAlertsConfig

	cancel context.CancelFunc

	mu sync.Mutex
	// Active incident lifecycle.
	incidentStartedAt time.Time
	incidentAlerted   bool
	incidentInitial   opsSnapshot
	incidentLatest    opsSnapshot
	incidentLastAlert opsSnapshot
	lastAlertAt       time.Time
}

type opsSnapshot struct {
	overallStatus       string
	reasonCode          string
	message             string
	lastSignalAgeSec    int64
	queueDepth          int64
	authFailures5m      int64
	contractMismatch5m  int64
	decisionReadyP95MS  int64
	decisionConsumeP95M int64
}

// NewOpsAlerts creates an ops alert worker with sane defaults.
func NewOpsAlerts(cfg OpsAlertsConfig) *OpsAlerts {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = 1500 * time.Millisecond
	}
	if cfg.DegradeFor <= 0 {
		cfg.DegradeFor = 20 * time.Second
	}
	if cfg.RepeatEvery <= 0 {
		cfg.RepeatEvery = 10 * time.Minute
	}
	if cfg.UpdateCooldown <= 0 {
		cfg.UpdateCooldown = 60 * time.Second
	}
	return &OpsAlerts{cfg: cfg}
}

// Start begins polling ops status.
func (oa *OpsAlerts) Start() {
	if oa.cfg.Probe == nil || oa.cfg.Send == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	oa.cancel = cancel

	go oa.loop(ctx)
}

// Stop stops ops polling.
func (oa *OpsAlerts) Stop() {
	if oa.cancel != nil {
		oa.cancel()
	}
}

func (oa *OpsAlerts) loop(ctx context.Context) {
	ticker := time.NewTicker(oa.cfg.PollInterval)
	defer ticker.Stop()

	// Evaluate immediately at startup, then on interval.
	oa.tick(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			oa.tick(ctx, time.Now().UTC())
		}
	}
}

func (oa *OpsAlerts) tick(parent context.Context, now time.Time) {
	probeCtx, cancel := context.WithTimeout(parent, oa.cfg.ProbeTimeout)
	defer cancel()

	payload, err := oa.cfg.Probe(probeCtx)
	snap := opsSnapshot{
		overallStatus: "RED",
		reasonCode:    "OPS_PROBE_FAILED",
		message:       errString(err),
	}
	if err == nil {
		if parsed, perr := parseOpsSnapshot(payload); perr == nil {
			snap = parsed
		} else {
			snap = opsSnapshot{
				overallStatus: "RED",
				reasonCode:    "OPS_PAYLOAD_INVALID",
				message:       perr.Error(),
			}
		}
	}

	alerts := oa.evaluate(now, snap)
	for _, msg := range alerts {
		oa.cfg.Send(parent, msg)
	}
}

func (oa *OpsAlerts) evaluate(now time.Time, snap opsSnapshot) []string {
	oa.mu.Lock()
	defer oa.mu.Unlock()

	status := strings.ToUpper(strings.TrimSpace(snap.overallStatus))
	if status == "" {
		status = "RED"
	}
	snap.overallStatus = status
	if strings.TrimSpace(snap.reasonCode) == "" {
		snap.reasonCode = "UNKNOWN"
	}

	// Healthy path.
	if status == "GREEN" {
		if oa.incidentStartedAt.IsZero() {
			return nil
		}
		if !oa.incidentAlerted {
			oa.resetIncident()
			return nil
		}
		duration := now.Sub(oa.incidentStartedAt)
		prev := oa.incidentLatest
		oa.resetIncident()
		return []string{formatRecoveryAlert(duration, prev)}
	}

	// Incident start.
	if oa.incidentStartedAt.IsZero() {
		oa.incidentStartedAt = now
		oa.incidentInitial = snap
		oa.incidentLatest = snap
		return nil
	}

	oa.incidentLatest = snap
	elapsed := now.Sub(oa.incidentStartedAt)
	changed := oa.incidentLastAlert.overallStatus != snap.overallStatus || oa.incidentLastAlert.reasonCode != snap.reasonCode

	// Debounced first alert for this incident.
	if !oa.incidentAlerted {
		if elapsed < oa.cfg.DegradeFor {
			return nil
		}
		oa.incidentAlerted = true
		oa.incidentLastAlert = snap
		oa.lastAlertAt = now
		return []string{formatDegradeAlert("degraded", elapsed, snap)}
	}

	// Immediate status/reason update with cooldown.
	if changed && now.Sub(oa.lastAlertAt) >= oa.cfg.UpdateCooldown {
		oa.incidentLastAlert = snap
		oa.lastAlertAt = now
		return []string{formatDegradeAlert("updated", elapsed, snap)}
	}

	// Periodic reminder while still degraded.
	if now.Sub(oa.lastAlertAt) >= oa.cfg.RepeatEvery {
		oa.incidentLastAlert = snap
		oa.lastAlertAt = now
		return []string{formatDegradeAlert("still_degraded", elapsed, snap)}
	}

	return nil
}

func (oa *OpsAlerts) resetIncident() {
	oa.incidentStartedAt = time.Time{}
	oa.incidentAlerted = false
	oa.incidentInitial = opsSnapshot{}
	oa.incidentLatest = opsSnapshot{}
	oa.incidentLastAlert = opsSnapshot{}
	oa.lastAlertAt = time.Time{}
}

func parseOpsSnapshot(payload map[string]any) (opsSnapshot, error) {
	if payload == nil {
		return opsSnapshot{}, fmt.Errorf("empty payload")
	}
	status := strings.TrimSpace(asString(payload["overall_status"], ""))
	reason := strings.TrimSpace(asString(payload["overall_reason_code"], ""))
	if status == "" {
		overall := asMap(payload["overall"])
		status = strings.TrimSpace(asString(overall["status"], ""))
		if reason == "" {
			reason = strings.TrimSpace(asString(overall["reason_code"], ""))
		}
	}
	if status == "" {
		return opsSnapshot{}, fmt.Errorf("missing overall status")
	}
	if reason == "" {
		reason = "UNKNOWN"
	}

	overall := asMap(payload["overall"])
	return opsSnapshot{
		overallStatus:       strings.ToUpper(status),
		reasonCode:          reason,
		message:             asString(overall["message"], ""),
		lastSignalAgeSec:    asInt64(payload["last_signal_age_sec"], -1),
		queueDepth:          asInt64(payload["queue_depth_active"], -1),
		authFailures5m:      asInt64(payload["auth_failures_5m"], -1),
		contractMismatch5m:  asInt64(payload["contract_mismatch_5m"], -1),
		decisionReadyP95MS:  asInt64(payload["decision_ready_p95_ms"], -1),
		decisionConsumeP95M: asInt64(payload["decision_consume_p95_ms"], -1),
	}, nil
}

func formatDegradeAlert(kind string, elapsed time.Duration, snap opsSnapshot) string {
	title := "🚨 *Ops Degraded*"
	if kind == "updated" {
		title = "⚠️ *Ops Status Update*"
	}
	if kind == "still_degraded" {
		title = "⚠️ *Ops Still Degraded*"
	}
	return fmt.Sprintf(
		"%s\n\nState: `%s`  Reason: `%s`\nDuration: %s\nSignalAge: %s  Queue: %s\nAuth401(5m): %s  Contract400(5m): %s\nDecision p95: %s / %s",
		title,
		snap.overallStatus,
		snap.reasonCode,
		humanDuration(elapsed),
		humanMetric(snap.lastSignalAgeSec, "s"),
		humanMetric(snap.queueDepth, ""),
		humanMetric(snap.authFailures5m, ""),
		humanMetric(snap.contractMismatch5m, ""),
		humanMetric(snap.decisionReadyP95MS, "ms"),
		humanMetric(snap.decisionConsumeP95M, "ms"),
	)
}

func formatRecoveryAlert(duration time.Duration, previous opsSnapshot) string {
	return fmt.Sprintf(
		"✅ *Ops Recovered*\n\nNow: `GREEN`\nPrevious: `%s` / `%s`\nIncident duration: %s",
		previous.overallStatus,
		previous.reasonCode,
		humanDuration(duration),
	)
}

func humanMetric(v int64, unit string) string {
	if v < 0 {
		return "n/a"
	}
	if unit == "" {
		return fmt.Sprintf("%d", v)
	}
	return fmt.Sprintf("%d%s", v, unit)
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int64(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int64(d.Minutes()), int64(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int64(d.Hours()), int64(d.Minutes())%60)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asString(v any, fallback string) string {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func asInt64(v any, fallback int64) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return fallback
	}
}
