package llm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// UsageRecord tracks token usage for a single LLM request.
type UsageRecord struct {
	Provider  string
	Tokens    int // approximate: len(prompt+response) / 4
	Latency   time.Duration
	Timestamp time.Time
}

// UsageTracker maintains per-provider usage statistics.
type UsageTracker struct {
	mu      sync.Mutex
	records []UsageRecord
}

// NewUsageTracker creates a new usage tracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		records: make([]UsageRecord, 0, 256),
	}
}

// Record logs a usage event.
func (u *UsageTracker) Record(provider string, inputChars, outputChars int, latency time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Approximate tokens: ~4 chars per token
	tokens := (inputChars + outputChars) / 4

	u.records = append(u.records, UsageRecord{
		Provider:  provider,
		Tokens:    tokens,
		Latency:   latency,
		Timestamp: time.Now(),
	})

	// Keep only last 24 hours of records
	cutoff := time.Now().Add(-24 * time.Hour)
	trimIdx := 0
	for trimIdx < len(u.records) && u.records[trimIdx].Timestamp.Before(cutoff) {
		trimIdx++
	}
	if trimIdx > 0 {
		u.records = u.records[trimIdx:]
	}
}

// ProviderStats holds aggregated usage for a single provider.
type ProviderStats struct {
	Provider     string
	Requests     int
	TotalTokens  int
	AvgLatencyMs int64
}

// DailyStats returns per-provider usage for the current day (UTC).
func (u *UsageTracker) DailyStats() []ProviderStats {
	u.mu.Lock()
	defer u.mu.Unlock()

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	byProvider := map[string]*ProviderStats{}

	for _, r := range u.records {
		if r.Timestamp.UTC().Before(todayStart) {
			continue
		}
		ps, ok := byProvider[r.Provider]
		if !ok {
			ps = &ProviderStats{Provider: r.Provider}
			byProvider[r.Provider] = ps
		}
		ps.Requests++
		ps.TotalTokens += r.Tokens
		ps.AvgLatencyMs += r.Latency.Milliseconds()
	}

	result := make([]ProviderStats, 0, len(byProvider))
	for _, ps := range byProvider {
		if ps.Requests > 0 {
			ps.AvgLatencyMs /= int64(ps.Requests)
		}
		result = append(result, *ps)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalTokens > result[j].TotalTokens
	})
	return result
}

// FormatUsage returns a human-readable usage summary for Telegram.
func (u *UsageTracker) FormatUsage() string {
	stats := u.DailyStats()
	if len(stats) == 0 {
		return "📊 *Usage Today*\n\n_No LLM requests recorded today._"
	}

	var sb strings.Builder
	sb.WriteString("📊 *Usage Today*\n\n")

	totalTokens := 0
	totalRequests := 0
	for _, s := range stats {
		sb.WriteString(fmt.Sprintf("• %s: %d requests, ~%d tokens, avg %dms\n",
			s.Provider, s.Requests, s.TotalTokens, s.AvgLatencyMs))
		totalTokens += s.TotalTokens
		totalRequests += s.Requests
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d requests, ~%d tokens", totalRequests, totalTokens))

	return sb.String()
}
