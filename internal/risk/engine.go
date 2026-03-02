package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/config"
)

// TradeProposal is what the agent submits to the risk engine for approval.
type TradeProposal struct {
	Symbol    string
	Direction string // BUY | SELL
	Lot       float64
	SL        float64
	TP        float64
	Entry     float64
}

// CheckResult holds the risk engine's verdict.
type CheckResult struct {
	Approved    bool
	AdjustedLot float64 // May be reduced by ramp-up
	Reason      string
}

// Engine enforces hard risk limits that the LLM cannot override (PRD §10).
type Engine struct {
	mu               sync.RWMutex
	cfg              config.RiskConfig
	dailyLoss        float64   // Running daily loss (USD), reset at 00:00 MYT
	openPositions    int       // Current open positions count
	lastTradeAt      time.Time // Timestamp of last trade execution
	profitableTrades int       // Count of profitable trades (for ramp-up)
	accountEquity    float64   // Updated by bridge
	halted           bool
}

// NewEngine creates a new risk engine from config.
func NewEngine(cfg config.RiskConfig) *Engine {
	return &Engine{
		cfg: cfg,
	}
}

// CheckTrade validates a trade proposal against all risk guardrails.
// Returns whether the trade is approved and the adjusted lot size.
func (e *Engine) CheckTrade(p TradeProposal) CheckResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// HALT check — nothing goes through
	if e.halted {
		return CheckResult{Approved: false, Reason: "HALT mode active — all trading frozen"}
	}

	// Max open positions
	if e.openPositions >= e.cfg.MaxOpenPositions {
		return CheckResult{Approved: false, Reason: fmt.Sprintf("max open positions reached (%d/%d)", e.openPositions, e.cfg.MaxOpenPositions)}
	}

	// Daily loss limit
	if e.dailyLoss >= e.cfg.MaxDailyLossUSD {
		return CheckResult{Approved: false, Reason: fmt.Sprintf("daily loss limit reached ($%.2f/$%.2f)", e.dailyLoss, e.cfg.MaxDailyLossUSD)}
	}

	// Drawdown circuit breaker
	if e.accountEquity > 0 {
		// Only check if we have equity data
		drawdownPct := (e.dailyLoss / e.accountEquity) * 100
		if drawdownPct >= e.cfg.MaxDrawdownPct {
			return CheckResult{Approved: false, Reason: fmt.Sprintf("drawdown limit reached (%.1f%%/%.1f%%)", drawdownPct, e.cfg.MaxDrawdownPct)}
		}
	}

	// Min trade interval — no overtrading
	if !e.lastTradeAt.IsZero() {
		elapsed := time.Since(e.lastTradeAt)
		minInterval := e.cfg.MinTradeIntervalDuration()
		if elapsed < minInterval {
			remaining := minInterval - elapsed
			return CheckResult{Approved: false, Reason: fmt.Sprintf("min trade interval — wait %s", remaining.Round(time.Second))}
		}
	}

	// Lot size validation + ramp-up
	lot := p.Lot
	if lot > e.cfg.MaxLotSize {
		lot = e.cfg.MaxLotSize
	}

	// Ramp-up: reduce lot to configured % until N profitable trades reached
	if e.profitableTrades < e.cfg.RampUpTrades {
		lot = lot * e.cfg.RampUpLotPct
	}

	if lot <= 0 {
		return CheckResult{Approved: false, Reason: "computed lot size is zero"}
	}

	return CheckResult{
		Approved:    true,
		AdjustedLot: lot,
		Reason:      "approved",
	}
}

// RecordTradeOpen increments open position count and sets last trade time.
func (e *Engine) RecordTradeOpen() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.openPositions++
	e.lastTradeAt = time.Now()
}

// RecordTradeClose decrements open positions and accumulates PnL.
func (e *Engine) RecordTradeClose(pnl float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.openPositions > 0 {
		e.openPositions--
	}
	if pnl < 0 {
		e.dailyLoss += -pnl // dailyLoss tracks absolute loss
	}
	if pnl > 0 {
		e.profitableTrades++
	}
}

// UpdateEquity sets the current account equity (called by bridge).
func (e *Engine) UpdateEquity(equity float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if equity > 0 {
		e.accountEquity = equity
	}
}

// SyncAccountSnapshot reconciles risk engine state with the latest MT5 snapshot.
// This is the source of truth after restarts or manual intervention in MT5.
func (e *Engine) SyncAccountSnapshot(equity float64, openPositions int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if equity > 0 {
		e.accountEquity = equity
	}
	if openPositions >= 0 {
		e.openPositions = openPositions
	}
}

// ResetDaily clears daily counters (called at 00:00 MYT).
func (e *Engine) ResetDaily() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dailyLoss = 0
}

// SetHalted enables or disables HALT mode.
func (e *Engine) SetHalted(halted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.halted = halted
}

// IsHalted returns whether the engine is in HALT state.
func (e *Engine) IsHalted() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.halted
}

// Stats returns current risk engine state for /status command.
type Stats struct {
	DailyLoss        float64
	OpenPositions    int
	MaxPositions     int
	ProfitableTrades int
	RampUpTarget     int
	Halted           bool
}

func (e *Engine) Stats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return Stats{
		DailyLoss:        e.dailyLoss,
		OpenPositions:    e.openPositions,
		MaxPositions:     e.cfg.MaxOpenPositions,
		ProfitableTrades: e.profitableTrades,
		RampUpTarget:     e.cfg.RampUpTrades,
		Halted:           e.halted,
	}
}
