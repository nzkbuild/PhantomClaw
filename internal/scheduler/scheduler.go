package scheduler

import (
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/nzkbuild/PhantomClaw/internal/config"
)

// SessionType identifies the current trading session.
type SessionType string

const (
	SessionLearning SessionType = "LEARNING" // 00:00-08:00 MYT
	SessionResearch SessionType = "RESEARCH" // 08:00-15:00 MYT
	SessionTrading  SessionType = "TRADING"  // 15:00-00:00 MYT
)

// Callbacks holds the functions triggered at each session transition.
type Callbacks struct {
	OnTokyoOpen  func() // 08:00 MYT — start data collection
	OnPreLondon  func() // 14:45 MYT — telegram alert
	OnLondonOpen func() // 15:00 MYT — trading mode starts
	OnHardStop   func() // 00:00 MYT — end of day, cancel pending, reset
}

// Scheduler manages MYT session windows and triggers callbacks (PRD §6).
type Scheduler struct {
	cron     *cron.Cron
	location *time.Location
	cfg      config.SessionConfig
}

// New creates a session scheduler anchored to the configured timezone.
func New(cfg config.SessionConfig, tz string, cb Callbacks) (*Scheduler, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("MYT", 8*60*60)
	}

	c := cron.New(cron.WithLocation(loc))

	// 08:00 MYT — Tokyo open → RESEARCH mode
	if cb.OnTokyoOpen != nil {
		spec := timeToCron(cfg.TokyoOpen)
		if _, err := c.AddFunc(spec, cb.OnTokyoOpen); err != nil {
			return nil, fmt.Errorf("scheduler: add tokyo_open: %w", err)
		}
		log.Printf("scheduler: registered tokyo_open at %s MYT", cfg.TokyoOpen)
	}

	// 14:45 MYT — Pre-London alert
	if cb.OnPreLondon != nil {
		spec := timeToCron(cfg.PreLondon)
		if _, err := c.AddFunc(spec, cb.OnPreLondon); err != nil {
			return nil, fmt.Errorf("scheduler: add pre_london: %w", err)
		}
		log.Printf("scheduler: registered pre_london at %s MYT", cfg.PreLondon)
	}

	// 15:00 MYT — London open → TRADING mode
	if cb.OnLondonOpen != nil {
		spec := timeToCron(cfg.LondonOpen)
		if _, err := c.AddFunc(spec, cb.OnLondonOpen); err != nil {
			return nil, fmt.Errorf("scheduler: add london_open: %w", err)
		}
		log.Printf("scheduler: registered london_open at %s MYT", cfg.LondonOpen)
	}

	// 00:00 MYT — Hard stop → cancel pending, reset daily counters
	if cb.OnHardStop != nil {
		spec := timeToCron(cfg.NYOverlapEnd)
		if _, err := c.AddFunc(spec, cb.OnHardStop); err != nil {
			return nil, fmt.Errorf("scheduler: add hard_stop: %w", err)
		}
		log.Printf("scheduler: registered hard_stop at %s MYT", cfg.NYOverlapEnd)
	}

	return &Scheduler{
		cron:     c,
		location: loc,
		cfg:      cfg,
	}, nil
}

// Start begins the scheduler (non-blocking).
func (s *Scheduler) Start() {
	s.cron.Start()
	log.Printf("scheduler: started — current session: %s", s.CurrentSession())
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// CurrentSession returns which session is active based on current MYT time.
func (s *Scheduler) CurrentSession() SessionType {
	now := time.Now().In(s.location)
	hour := now.Hour()
	minute := now.Minute()
	timeMinutes := hour*60 + minute

	switch {
	case timeMinutes >= 0 && timeMinutes < 480: // 00:00 - 08:00
		return SessionLearning
	case timeMinutes >= 480 && timeMinutes < 900: // 08:00 - 15:00
		return SessionResearch
	default: // 15:00 - 00:00
		return SessionTrading
	}
}

// IsWeekend returns true if current day is Saturday or Sunday (no trading).
func (s *Scheduler) IsWeekend() bool {
	now := time.Now().In(s.location)
	day := now.Weekday()
	return day == time.Saturday || day == time.Sunday
}

// timeToCron converts "HH:MM" to a cron spec "M H * * 1-5" (weekdays only).
func timeToCron(hhmm string) string {
	var h, m int
	fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return fmt.Sprintf("%d %d * * 1-5", m, h) // weekdays only
}
