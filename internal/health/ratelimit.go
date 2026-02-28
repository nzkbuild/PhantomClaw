package health

import (
	"sync"
	"time"
)

// RateLimiter enforces per-endpoint request rate limits for TOS compliance (PRD §18).
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time // endpoint -> timestamps
	limits   map[string]rateLimit
}

type rateLimit struct {
	maxRequests int
	window      time.Duration
}

// NewRateLimiter creates a rate limiter with default limits for external APIs.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limits: map[string]rateLimit{
			"forexfactory": {maxRequests: 10, window: time.Minute},     // 10 req/min
			"reddit":       {maxRequests: 30, window: time.Minute},     // 30 req/min
			"cftc":         {maxRequests: 5, window: 10 * time.Minute}, // 5 req/10min
			"twelvedata":   {maxRequests: 8, window: time.Minute},      // 8 req/min (free tier)
			"llm":          {maxRequests: 60, window: time.Minute},     // 60 req/min
		},
	}
}

// Allow checks if a request to the endpoint is allowed under rate limits.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow(endpoint string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit, ok := rl.limits[endpoint]
	if !ok {
		return true // No limit configured
	}

	now := time.Now()
	cutoff := now.Add(-limit.window)

	// Clean old timestamps
	var recent []time.Time
	for _, t := range rl.requests[endpoint] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	rl.requests[endpoint] = recent

	if len(recent) >= limit.maxRequests {
		return false
	}

	rl.requests[endpoint] = append(rl.requests[endpoint], now)
	return true
}

// WaitTime returns how long to wait before the next request is allowed.
func (rl *RateLimiter) WaitTime(endpoint string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit, ok := rl.limits[endpoint]
	if !ok {
		return 0
	}

	now := time.Now()
	cutoff := now.Add(-limit.window)

	var recent []time.Time
	for _, t := range rl.requests[endpoint] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) < limit.maxRequests {
		return 0
	}

	// Wait until the oldest request expires
	return recent[0].Add(limit.window).Sub(now)
}
