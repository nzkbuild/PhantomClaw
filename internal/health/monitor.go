package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status represents a component's health state.
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

// ComponentCheck is a function that returns the health of a component.
type ComponentCheck func() Status

// AlertFunc is called when a health check fails.
type AlertFunc func(component string, status Status, message string)

// Monitor tracks health of all subsystems and sends periodic Telegram pings.
type Monitor struct {
	mu         sync.RWMutex
	checks     map[string]ComponentCheck
	lastStatus map[string]Status
	interval   time.Duration
	onAlert    AlertFunc
	cancel     context.CancelFunc
}

// NewMonitor creates a health monitor with the given check interval.
func NewMonitor(interval time.Duration, onAlert AlertFunc) *Monitor {
	return &Monitor{
		checks:     make(map[string]ComponentCheck),
		lastStatus: make(map[string]Status),
		interval:   interval,
		onAlert:    onAlert,
	}
}

// Register adds a named health check.
func (m *Monitor) Register(name string, check ComponentCheck) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checks[name] = check
	m.lastStatus[name] = StatusOK
}

// Start begins periodic health checking (non-blocking).
func (m *Monitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.runChecks()
			}
		}
	}()
}

// Stop halts health monitoring.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// runChecks evaluates all registered health checks.
func (m *Monitor) runChecks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, check := range m.checks {
		status := check()
		prev := m.lastStatus[name]

		// Alert on status change
		if status != prev && status != StatusOK {
			if m.onAlert != nil {
				m.onAlert(name, status, fmt.Sprintf("%s changed from %s to %s", name, prev, status))
			}
		}
		m.lastStatus[name] = status
	}
}

// Summary returns the current health status of all components.
func (m *Monitor) Summary() map[string]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]Status)
	for name := range m.checks {
		result[name] = m.lastStatus[name]
	}
	return result
}

// IsHealthy returns true if all components are OK.
func (m *Monitor) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, status := range m.lastStatus {
		if status != StatusOK {
			return false
		}
	}
	return true
}

// StatusText returns a formatted health status for Telegram.
func (m *Monitor) StatusText() string {
	summary := m.Summary()
	emoji := map[Status]string{
		StatusOK:       "🟢",
		StatusDegraded: "🟡",
		StatusDown:     "🔴",
	}

	text := "🏥 *Health Check*\n\n"
	for name, status := range summary {
		e := emoji[status]
		if e == "" {
			e = "⚪"
		}
		text += fmt.Sprintf("%s %s: %s\n", e, name, status)
	}
	return text
}
