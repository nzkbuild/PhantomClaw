package alerts

import (
	"context"
	"fmt"
	"time"
)

// TelegramSender is a function that sends a message via Telegram.
type TelegramSender func(ctx context.Context, text string)

// SessionAlerts manages scheduled Telegram notifications (PRD §18).
type SessionAlerts struct {
	send     TelegramSender
	location *time.Location
	cancel   context.CancelFunc
}

// NewSessionAlerts creates a session alert manager.
func NewSessionAlerts(send TelegramSender, tz string) *SessionAlerts {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("MYT", 8*60*60)
	}
	return &SessionAlerts{send: send, location: loc}
}

// Start begins the alert loop (non-blocking).
func (sa *SessionAlerts) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	sa.cancel = cancel

	go sa.loop(ctx)
}

// Stop halts alerts.
func (sa *SessionAlerts) Stop() {
	if sa.cancel != nil {
		sa.cancel()
	}
}

// loop runs the alert scheduler.
func (sa *SessionAlerts) loop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	lastPing := time.Time{}
	sentToday := map[string]bool{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().In(sa.location)
			hhmm := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
			today := now.Format("2006-01-02")

			// Reset daily alerts at midnight
			if hhmm == "00:00" {
				sentToday = map[string]bool{}
			}

			// Skip weekends
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				continue
			}

			// Morning brief (08:00 MYT)
			if hhmm == "08:00" && !sentToday["morning_"+today] {
				sa.send(ctx, "☀️ *Morning Brief*\n\nTokyo session opening. RESEARCH mode active.\nData collection and MTF analysis starting.")
				sentToday["morning_"+today] = true
			}

			// Pre-London alert (14:45 MYT)
			if hhmm == "14:45" && !sentToday["prelondon_"+today] {
				sa.send(ctx, "⚡ *15 min to London Open*\n\nPrepare for TRADING mode.\nPending orders should be in place.")
				sentToday["prelondon_"+today] = true
			}

			// London open (15:00 MYT)
			if hhmm == "15:00" && !sentToday["london_"+today] {
				sa.send(ctx, "🇬🇧 *London Open*\n\nTRADING mode active.\nMonitoring pending orders.")
				sentToday["london_"+today] = true
			}

			// NY overlap (20:00 MYT)
			if hhmm == "20:00" && !sentToday["nyoverlap_"+today] {
				sa.send(ctx, "🇺🇸 *NY Session Overlap*\n\nPeak liquidity. Watch for high-impact moves.")
				sentToday["nyoverlap_"+today] = true
			}

			// 6-hour health ping
			if time.Since(lastPing) >= 6*time.Hour {
				sa.send(ctx, fmt.Sprintf("💓 *Health Ping*\n\nPhantomClaw running. Time: %s MYT", now.Format("15:04")))
				lastPing = now
			}
		}
	}
}
