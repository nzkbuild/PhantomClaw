package scheduler

import (
	"log"
	"time"
)

// HeartbeatConfig configures the periodic health check.
type HeartbeatConfig struct {
	IntervalMin int              // heartbeat every N minutes (default: 5)
	HealthCheck func() error     // function to check system health (e.g. DB ping)
	Alerter     func(msg string) // function to send alerts (e.g. Telegram)
}

// Heartbeat performs periodic health checks and alerts on failure.
type Heartbeat struct {
	cfg    HeartbeatConfig
	ticker *time.Ticker
	stop   chan struct{}
}

// NewHeartbeat creates a heartbeat worker.
func NewHeartbeat(cfg HeartbeatConfig) *Heartbeat {
	if cfg.IntervalMin <= 0 {
		cfg.IntervalMin = 5
	}
	return &Heartbeat{
		cfg:  cfg,
		stop: make(chan struct{}),
	}
}

// Start launches the heartbeat in a background goroutine.
func (h *Heartbeat) Start() {
	h.ticker = time.NewTicker(time.Duration(h.cfg.IntervalMin) * time.Minute)
	log.Printf("heartbeat: started (every %d min)", h.cfg.IntervalMin)

	go func() {
		for {
			select {
			case <-h.ticker.C:
				h.check()
			case <-h.stop:
				return
			}
		}
	}()
}

// Stop shuts down the heartbeat.
func (h *Heartbeat) Stop() {
	if h.ticker != nil {
		h.ticker.Stop()
	}
	close(h.stop)
}

func (h *Heartbeat) check() {
	if h.cfg.HealthCheck == nil {
		return
	}

	if err := h.cfg.HealthCheck(); err != nil {
		log.Printf("heartbeat: health check FAILED: %v", err)
		if h.cfg.Alerter != nil {
			h.cfg.Alerter("⚠️ PhantomClaw health check failed: " + err.Error())
		}
	}
}
