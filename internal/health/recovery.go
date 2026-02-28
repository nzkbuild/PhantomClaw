package health

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Recovery handles error tracking and automatic recovery actions.
type Recovery struct {
	mu          sync.Mutex
	errorCounts map[string]int       // component -> consecutive error count
	lastError   map[string]time.Time // component -> last error time
	thresholds  map[string]int       // component -> max errors before action
	onRecovery  func(component, action string)
}

// NewRecovery creates an error recovery handler.
func NewRecovery(onRecovery func(component, action string)) *Recovery {
	return &Recovery{
		errorCounts: make(map[string]int),
		lastError:   make(map[string]time.Time),
		thresholds: map[string]int{
			"llm":      3,  // 3 consecutive LLM errors → switch provider
			"bridge":   5,  // 5 bridge errors → HALT mode
			"telegram": 10, // 10 telegram errors → log only
			"memory":   2,  // 2 DB errors → HALT mode
		},
		onRecovery: onRecovery,
	}
}

// RecordError increments the error counter for a component.
// Returns the recovery action if threshold is exceeded.
func (r *Recovery) RecordError(component string, err error) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errorCounts[component]++
	r.lastError[component] = time.Now()
	count := r.errorCounts[component]

	threshold, ok := r.thresholds[component]
	if !ok {
		threshold = 5 // default
	}

	if count >= threshold {
		action := r.determineAction(component, count)
		log.Printf("recovery: %s hit threshold (%d errors) → %s", component, count, action)
		if r.onRecovery != nil {
			r.onRecovery(component, action)
		}
		return action
	}

	return ""
}

// RecordSuccess resets the error counter for a component.
func (r *Recovery) RecordSuccess(component string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorCounts[component] = 0
}

// determineAction decides what to do when a component exceeds its error threshold.
func (r *Recovery) determineAction(component string, count int) string {
	switch component {
	case "llm":
		return "switch_provider"
	case "bridge":
		if count >= 10 {
			return "halt"
		}
		return "reconnect"
	case "memory":
		return "halt"
	case "telegram":
		return "log_only"
	default:
		return "log_only"
	}
}

// Stats returns error tracking info for diagnostics.
func (r *Recovery) Stats() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]string)
	for comp, count := range r.errorCounts {
		last := r.lastError[comp]
		result[comp] = fmt.Sprintf("errors=%d, last=%s", count, last.Format(time.RFC3339))
	}
	return result
}
