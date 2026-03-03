package safety

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Mode represents the bot's operational safety mode (PRD §9).
type Mode int

const (
	ModeObserve Mode = iota // Analyzes and reports only. Zero execution.
	ModeSuggest             // Proposes trade → waits for Telegram approval.
	ModeAuto                // Executes autonomously within hard risk limits.
	ModeHalt                // Closes all positions, freezes everything.
)

// String returns the human-readable mode name.
func (m Mode) String() string {
	switch m {
	case ModeObserve:
		return "OBSERVE"
	case ModeSuggest:
		return "SUGGEST"
	case ModeAuto:
		return "AUTO"
	case ModeHalt:
		return "HALT"
	default:
		return "UNKNOWN"
	}
}

// ParseMode converts a string to a Mode.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "OBSERVE", "observe":
		return ModeObserve, nil
	case "SUGGEST", "suggest":
		return ModeSuggest, nil
	case "AUTO", "auto":
		return ModeAuto, nil
	case "HALT", "halt":
		return ModeHalt, nil
	default:
		return ModeObserve, fmt.Errorf("unknown mode: %q (valid: OBSERVE, SUGGEST, AUTO, HALT)", s)
	}
}

// SessionWindow defines a time range for session enforcement.
type SessionWindow struct {
	LearningStart string // "00:00" MYT
	LearningEnd   string // "08:00" MYT
}

// Manager handles mode switching with thread-safety and session-aware enforcement.
// Outside trading hours, mode is forced to OBSERVE regardless of user setting (PRD §9).
type Manager struct {
	mu                sync.RWMutex
	currentMode       Mode
	userSetMode       Mode // What user requested — restored after session override
	sessionWindow     SessionWindow
	learningStartMins int
	learningEndMins   int
	hasLearningWindow bool
	location          *time.Location
	onHalt            func() // Callback when HALT is triggered
	nowFn             func() time.Time
}

// NewManager creates a mode manager with the default mode and session windows.
func NewManager(defaultMode string, sw SessionWindow, tz string, onHalt func()) (*Manager, error) {
	mode, err := ParseMode(defaultMode)
	if err != nil {
		mode = ModeAuto // fallback to PRD default
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("MYT", 8*60*60) // UTC+8 fallback
	}

	startMins, startOK := parseHHMMToMinutes(sw.LearningStart)
	endMins, endOK := parseHHMMToMinutes(sw.LearningEnd)

	m := &Manager{
		currentMode:   mode,
		userSetMode:   mode,
		sessionWindow: sw,
		location:      loc,
		onHalt:        onHalt,
		nowFn:         time.Now,
	}

	if startOK && endOK {
		m.learningStartMins = startMins
		m.learningEndMins = endMins
		m.hasLearningWindow = true
	} else {
		// Backward-compatible fallback if config is invalid/missing.
		m.learningStartMins = 0
		m.learningEndMins = 8 * 60
		m.hasLearningWindow = true
	}

	return m, nil
}

// CurrentMode returns the effective mode, accounting for session-aware override.
func (m *Manager) CurrentMode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// HALT always overrides everything
	if m.currentMode == ModeHalt {
		return ModeHalt
	}

	// Outside trading hours → force OBSERVE
	if m.isLearningHours() {
		return ModeObserve
	}

	return m.currentMode
}

// SetMode switches the mode. HALT triggers the onHalt callback.
func (m *Manager) SetMode(mode Mode) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentMode = mode
	if mode != ModeHalt {
		m.userSetMode = mode // Remember user preference
	}

	if mode == ModeHalt && m.onHalt != nil {
		go m.onHalt() // Non-blocking callback
	}
}

// ExitHalt restores the previous user-set mode.
func (m *Manager) ExitHalt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentMode = m.userSetMode
}

// CanTrade returns true if the current effective mode allows trade execution.
func (m *Manager) CanTrade() bool {
	mode := m.CurrentMode()
	return mode == ModeAuto || mode == ModeSuggest
}

// CanExecuteAutonomously returns true if AUTO mode is active.
func (m *Manager) CanExecuteAutonomously() bool {
	return m.CurrentMode() == ModeAuto
}

// isLearningHours checks if current MYT time is in LEARNING window (00:00-08:00).
func (m *Manager) isLearningHours() bool {
	if !m.hasLearningWindow {
		return false
	}

	now := m.now()
	minutes := now.Hour()*60 + now.Minute()
	start := m.learningStartMins
	end := m.learningEndMins

	// Equal start/end means 24h learning window.
	if start == end {
		return true
	}
	// Normal same-day window (e.g., 00:00-08:00)
	if start < end {
		return minutes >= start && minutes < end
	}
	// Wrap-around window across midnight (e.g., 22:00-06:00)
	return minutes >= start || minutes < end
}

// StatusText returns a formatted status string for Telegram.
func (m *Manager) StatusText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	effective := m.CurrentMode()
	userSet := m.userSetMode

	if m.isLearningHours() && m.currentMode != ModeHalt {
		return fmt.Sprintf("🔵 %s (off-hours override, user set: %s)", effective, userSet)
	}
	switch effective {
	case ModeHalt:
		return "🛑 HALT — all trading frozen"
	case ModeAuto:
		return "🟢 AUTO — trading autonomously"
	case ModeSuggest:
		return "🟡 SUGGEST — proposing trades for approval"
	case ModeObserve:
		return "⚪ OBSERVE — analysis only, no execution"
	default:
		return effective.String()
	}
}

func (m *Manager) now() time.Time {
	if m.nowFn == nil {
		return time.Now().In(m.location)
	}
	return m.nowFn().In(m.location)
}

func parseHHMMToMinutes(v string) (int, bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}
