package skills

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
)

// maxDynamicJobs limits how many agent-scheduled jobs can exist at once.
const maxDynamicJobs = 5

// CronDeps holds dependencies for the cron_add skill.
type CronDeps struct {
	Scheduler *scheduler.Scheduler
	OnWake    func(pair, reason string) // called when a scheduled check fires
}

// cronTracker keeps track of active dynamic jobs to enforce limits.
type cronTracker struct {
	mu    sync.Mutex
	count int
}

var tracker = &cronTracker{}

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

			// Check limits
			tracker.mu.Lock()
			if tracker.count >= maxDynamicJobs {
				tracker.mu.Unlock()
				return `{"error":"max dynamic jobs reached (5), wait for existing ones to fire"}`, nil
			}
			tracker.count++
			tracker.mu.Unlock()

			// Schedule one-shot wake-up
			wakeAt := time.Now().Add(time.Duration(p.DelayMinutes) * time.Minute)
			jobName := fmt.Sprintf("recheck_%s_%s", p.Pair, wakeAt.Format("15:04"))
			pair := p.Pair
			reason := p.Reason

			// One-shot timer: fire once after N minutes.
			time.AfterFunc(time.Duration(p.DelayMinutes)*time.Minute, func() {
				log.Printf("cron_add: firing recheck for %s — %s", pair, reason)
				if deps.OnWake != nil {
					deps.OnWake(pair, reason)
				}
				tracker.mu.Lock()
				tracker.count--
				tracker.mu.Unlock()
			})

			result := map[string]any{
				"status":   "scheduled",
				"pair":     p.Pair,
				"wake_at":  wakeAt.Format("15:04 MYT"),
				"delay":    p.DelayMinutes,
				"reason":   p.Reason,
				"job_name": jobName,
			}
			data, _ := json.Marshal(result)
			return string(data), nil
		},
	}
}
