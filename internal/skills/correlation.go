package skills

import (
	"fmt"
	"sync"
)

// CorrelationPair defines two symbols that are correlated.
type CorrelationPair struct {
	A       string  // e.g. "EURUSD"
	B       string  // e.g. "GBPUSD"
	Corr    float64 // 0.0 to 1.0 — absolute correlation strength
	SameDir bool    // true = positive correlation (both move same way)
}

// CorrelationGuard blocks conflicting positions on correlated pairs (PRD §14).
// Example: BUY EURUSD + SELL GBPUSD when they are positively correlated = blocked.
type CorrelationGuard struct {
	mu        sync.RWMutex
	pairs     []CorrelationPair
	openPos   map[string]string // symbol -> direction (BUY/SELL)
	threshold float64
}

// NewCorrelationGuard creates a guard with default forex correlation pairs.
func NewCorrelationGuard(threshold float64) *CorrelationGuard {
	if threshold <= 0 {
		threshold = 0.7 // Default: block if correlation > 70%
	}

	return &CorrelationGuard{
		pairs: []CorrelationPair{
			{A: "EURUSD", B: "GBPUSD", Corr: 0.85, SameDir: true},
			{A: "EURUSD", B: "USDCHF", Corr: 0.90, SameDir: false},
			{A: "AUDUSD", B: "NZDUSD", Corr: 0.88, SameDir: true},
			{A: "USDJPY", B: "USDCHF", Corr: 0.75, SameDir: true},
			{A: "GBPUSD", B: "USDCHF", Corr: 0.80, SameDir: false},
		},
		openPos:   make(map[string]string),
		threshold: threshold,
	}
}

// Check returns an error if the proposed trade conflicts with an open position on a correlated pair.
func (cg *CorrelationGuard) Check(symbol, direction string) error {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	for _, cp := range cg.pairs {
		if cp.Corr < cg.threshold {
			continue
		}

		var correlated string
		var isSymbolA bool
		if cp.A == symbol {
			correlated = cp.B
			isSymbolA = true
		} else if cp.B == symbol {
			correlated = cp.A
			isSymbolA = false
		} else {
			continue
		}

		openDir, hasOpen := cg.openPos[correlated]
		if !hasOpen {
			continue
		}

		// Check for conflict
		conflicting := false
		if cp.SameDir {
			// Positive correlation: same direction on both = OK, opposite = conflict
			conflicting = (direction != openDir)
		} else {
			// Negative correlation: opposite direction on both = OK, same = conflict
			conflicting = (direction == openDir)
			_ = isSymbolA // direction logic is symmetric for inverse pairs
		}

		if conflicting {
			return fmt.Errorf("correlation guard: %s %s conflicts with open %s %s (corr=%.0f%%)",
				direction, symbol, openDir, correlated, cp.Corr*100)
		}
	}
	return nil
}

// RecordOpen tracks an opened position.
func (cg *CorrelationGuard) RecordOpen(symbol, direction string) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.openPos[symbol] = direction
}

// RecordClose removes a closed position from tracking.
func (cg *CorrelationGuard) RecordClose(symbol string) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	delete(cg.openPos, symbol)
}
