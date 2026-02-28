package skills

import (
	"fmt"
	"sync"
)

// SpreadFilter tracks rolling average spreads and rejects entries > 2x average (PRD §14).
type SpreadFilter struct {
	mu          sync.RWMutex
	spreads     map[string][]float64 // symbol -> last N spreads
	maxN        int                  // rolling window size
	maxMultiple float64              // reject if current > maxMultiple * avg
}

// NewSpreadFilter creates a spread filter.
func NewSpreadFilter(windowSize int, maxMultiple float64) *SpreadFilter {
	if windowSize <= 0 {
		windowSize = 50
	}
	if maxMultiple <= 0 {
		maxMultiple = 2.0
	}
	return &SpreadFilter{
		spreads:     make(map[string][]float64),
		maxN:        windowSize,
		maxMultiple: maxMultiple,
	}
}

// Record adds a spread observation for a symbol.
func (sf *SpreadFilter) Record(symbol string, spread float64) {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	sf.spreads[symbol] = append(sf.spreads[symbol], spread)
	if len(sf.spreads[symbol]) > sf.maxN {
		sf.spreads[symbol] = sf.spreads[symbol][1:]
	}
}

// Check returns an error if the current spread is too wide.
func (sf *SpreadFilter) Check(symbol string, currentSpread float64) error {
	sf.mu.RLock()
	defer sf.mu.RUnlock()

	history, ok := sf.spreads[symbol]
	if !ok || len(history) < 10 {
		// Not enough data to judge — allow trade
		return nil
	}

	avg := average(history)
	if avg <= 0 {
		return nil
	}

	ratio := currentSpread / avg
	if ratio > sf.maxMultiple {
		return fmt.Errorf("spread filter: %s spread %.1f is %.1fx average (%.1f), threshold %.1fx",
			symbol, currentSpread, ratio, avg, sf.maxMultiple)
	}
	return nil
}

// AverageSpread returns the rolling average spread for a symbol.
func (sf *SpreadFilter) AverageSpread(symbol string) float64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return average(sf.spreads[symbol])
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
