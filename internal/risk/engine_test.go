package risk

import (
	"testing"

	"github.com/nzkbuild/PhantomClaw/internal/config"
)

func testRiskConfig() config.RiskConfig {
	return config.RiskConfig{
		MaxLotSize:       1.0,
		MaxDailyLossUSD:  10000,
		MaxOpenPositions: 3,
		MaxDrawdownPct:   5.0,
		MinTradeInterval: "0s",
		RampUpTrades:     0,
		RampUpLotPct:     1.0,
	}
}

func testProposal() TradeProposal {
	return TradeProposal{
		Symbol:    "EURUSD",
		Direction: "BUY",
		Lot:       0.1,
		SL:        1.0900,
		TP:        1.1100,
		Entry:     1.1000,
	}
}

func TestRiskSnapshotReconciliation(t *testing.T) {
	e := NewEngine(testRiskConfig())
	p := testProposal()

	// Fill current position count to max, then reconcile down from MT5 snapshot.
	e.RecordTradeOpen()
	e.RecordTradeOpen()
	e.RecordTradeOpen()
	blocked := e.CheckTrade(p)
	if blocked.Approved {
		t.Fatal("expected trade blocked at max open positions")
	}

	e.SyncAccountSnapshot(2500.0, 0)
	approved := e.CheckTrade(p)
	if !approved.Approved {
		t.Fatalf("expected trade approved after snapshot reconciliation, got reason=%q", approved.Reason)
	}

	// Reconcile up from MT5 snapshot and ensure gate blocks again.
	e.SyncAccountSnapshot(2500.0, 3)
	blockedAgain := e.CheckTrade(p)
	if blockedAgain.Approved {
		t.Fatal("expected trade blocked when reconciled open positions are at max")
	}
}

func TestRiskSnapshotEquityAffectsDrawdownCheck(t *testing.T) {
	e := NewEngine(testRiskConfig())
	p := testProposal()

	// Daily loss is tracked from closed losing trades.
	e.RecordTradeClose(-100.0) // dailyLoss = 100
	e.SyncAccountSnapshot(1000.0, 0)

	// 100/1000 = 10% drawdown > 5% max => must block.
	result := e.CheckTrade(p)
	if result.Approved {
		t.Fatal("expected drawdown gate to block after equity sync")
	}
}
