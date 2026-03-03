package skills

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
)

// maxDynamicJobs limits how many agent-scheduled jobs can exist at once.
const maxDynamicJobs = 5

// CronDeps holds dependencies for the cron_add skill.
type CronDeps struct {
	Scheduler *scheduler.Scheduler
	OnWake    func(pair, reason string) // called when a scheduled check fires
	DB        *memory.DB
}

// cronTracker keeps track of active dynamic jobs to enforce limits.
type cronTracker struct {
	mu     sync.Mutex
	count  int
	active map[string]struct{}
}

var tracker = &cronTracker{active: make(map[string]struct{})}

func (t *cronTracker) reserve(jobID string, enforceLimit bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.active[jobID]; exists {
		return false
	}
	if enforceLimit && t.count >= maxDynamicJobs {
		return false
	}

	t.active[jobID] = struct{}{}
	t.count++
	return true
}

func (t *cronTracker) release(jobID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.active[jobID]; exists {
		delete(t.active, jobID)
		if t.count > 0 {
			t.count--
		}
	}
}

func scheduleReservedOneShot(jobID, pair, reason string, wakeAt time.Time, deps CronDeps) {
	delay := time.Until(wakeAt)
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		log.Printf("cron_add: firing recheck for %s — %s (job=%s)", pair, reason, jobID)
		if deps.DB != nil {
			_ = deps.DB.MarkCronJobFired(jobID)
		}
		if deps.OnWake != nil {
			deps.OnWake(pair, reason)
		}
		tracker.release(jobID)
	})
}

// ReplayPendingCronJobs restores durable cron_add jobs from DB and re-schedules them.
func ReplayPendingCronJobs(deps CronDeps) error {
	if deps.DB == nil {
		return nil
	}

	jobs, err := deps.DB.ListPendingCronJobs()
	if err != nil {
		return fmt.Errorf("cron_add: replay list pending jobs: %w", err)
	}

	for _, job := range jobs {
		if !tracker.reserve(job.JobID, false) {
			continue
		}
		scheduleReservedOneShot(job.JobID, job.Pair, job.Reason, job.WakeAt, deps)
	}
	return nil
}

// CronAddSkill creates the cron_add tool for agent self-scheduling.
func CronAddSkill(deps CronDeps) *Skill {
	return &Skill{
		Name:        "cron_add",
		Description: "Schedule a future recheck — 'wake me in N minutes to recheck a pair'. Use when current conditions are uncertain and you want to revisit later.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pair": map[string]any{
					"type":        "string",
					"description": "Trading pair to recheck, e.g. EURUSD, XAUUSD",
				},
				"delay_minutes": map[string]any{
					"type":        "integer",
					"description": "Minutes from now to recheck (1-480)",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Why you want to recheck (logged for context)",
				},
			},
			"required": []string{"pair", "delay_minutes", "reason"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Pair         string `json:"pair"`
				DelayMinutes int    `json:"delay_minutes"`
				Reason       string `json:"reason"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("cron_add: invalid args: %w", err)
			}

			// Validate
			if p.DelayMinutes < 1 || p.DelayMinutes > 480 {
				return `{"error":"delay_minutes must be 1-480"}`, nil
			}

			// Reserve slot with limit enforcement.
			wakeAt := time.Now().Add(time.Duration(p.DelayMinutes) * time.Minute)
			jobID := fmt.Sprintf("recheck_%s_%d", p.Pair, wakeAt.UnixNano())
			if !tracker.reserve(jobID, true) {
				return `{"error":"max dynamic jobs reached (5), wait for existing ones to fire"}`, nil
			}

			pair := p.Pair
			reason := p.Reason

			if deps.DB != nil {
				if err := deps.DB.UpsertCronJob(jobID, pair, reason, wakeAt); err != nil {
					tracker.release(jobID)
					return "", fmt.Errorf("cron_add: persist job: %w", err)
				}
			}

			// One-shot timer: fire once after N minutes.
			scheduleReservedOneShot(jobID, pair, reason, wakeAt, deps)

			result := map[string]any{
				"status":   "scheduled",
				"pair":     p.Pair,
				"wake_at":  wakeAt.Format("15:04 MYT"),
				"delay":    p.DelayMinutes,
				"reason":   p.Reason,
				"job_name": jobID,
			}
			data, _ := json.Marshal(result)
			return string(data), nil
		},
	}
}
