package memory

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPendingDecisionLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Upsert and fetch by request_id.
	err = db.UpsertPendingDecision("req-1", "EURUSD", `{"action":"PLACE_PENDING"}`, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("UpsertPendingDecision: %v", err)
	}

	decisionJSON, status, found, err := db.GetPendingDecisionByRequestID("req-1")
	if err != nil {
		t.Fatalf("GetPendingDecisionByRequestID: %v", err)
	}
	if !found {
		t.Fatal("expected decision for req-1")
	}
	if decisionJSON == "" {
		t.Fatal("expected non-empty decision json")
	}
	if status != "pending" {
		t.Fatalf("status=%q, want=pending", status)
	}

	// Fetch by symbol.
	requestID, bySymbolJSON, bySymbolStatus, found, err := db.GetPendingDecisionBySymbol("EURUSD")
	if err != nil {
		t.Fatalf("GetPendingDecisionBySymbol: %v", err)
	}
	if !found || requestID != "req-1" || bySymbolJSON == "" || bySymbolStatus != "pending" {
		t.Fatalf("unexpected symbol fetch: found=%v request_id=%q status=%q json=%q", found, requestID, bySymbolStatus, bySymbolJSON)
	}

	// Mark delivered and verify it is still retrievable as active.
	if err := db.MarkPendingDecisionDelivered("req-1"); err != nil {
		t.Fatalf("MarkPendingDecisionDelivered: %v", err)
	}
	_, status, found, err = db.GetPendingDecisionByRequestID("req-1")
	if err != nil {
		t.Fatalf("GetPendingDecisionByRequestID after delivered: %v", err)
	}
	if !found || status != "delivered" {
		t.Fatalf("expected delivered state, found=%v status=%q", found, status)
	}

	// Consume and verify no longer pending.
	if err := db.ConsumePendingDecision("req-1"); err != nil {
		t.Fatalf("ConsumePendingDecision: %v", err)
	}
	_, _, found, err = db.GetPendingDecisionByRequestID("req-1")
	if err != nil {
		t.Fatalf("GetPendingDecisionByRequestID after consume: %v", err)
	}
	if found {
		t.Fatal("expected req-1 to be consumed")
	}

	var queryStatus string
	if err := db.QueryRow("SELECT status FROM pending_decisions WHERE request_id = ?", "req-1").Scan(&queryStatus); err != nil {
		t.Fatalf("read consumed status: %v", err)
	}
	if queryStatus != "consumed" {
		t.Fatalf("status=%q, want=consumed", queryStatus)
	}

	// Insert an already-expired pending decision, then expire sweep.
	err = db.UpsertPendingDecision("req-expired", "XAUUSD", `{"action":"HOLD"}`, time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("UpsertPendingDecision expired: %v", err)
	}
	if err := db.ExpirePendingDecisions(time.Now()); err != nil {
		t.Fatalf("ExpirePendingDecisions: %v", err)
	}

	if err := db.QueryRow("SELECT status FROM pending_decisions WHERE request_id = ?", "req-expired").Scan(&queryStatus); err != nil {
		t.Fatalf("read expired status: %v", err)
	}
	if queryStatus != "expired" {
		t.Fatalf("status=%q, want=expired", queryStatus)
	}
}

func TestCronJobLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	wakeAt := time.Now().Add(2 * time.Minute)
	if err := db.UpsertCronJob("job-1", "EURUSD", "recheck trend", wakeAt); err != nil {
		t.Fatalf("UpsertCronJob: %v", err)
	}

	jobs, err := db.ListPendingCronJobs()
	if err != nil {
		t.Fatalf("ListPendingCronJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs)=%d, want=1", len(jobs))
	}
	if jobs[0].JobID != "job-1" || jobs[0].Pair != "EURUSD" || jobs[0].Status != "pending" {
		t.Fatalf("unexpected job: %+v", jobs[0])
	}

	if err := db.MarkCronJobFired("job-1"); err != nil {
		t.Fatalf("MarkCronJobFired: %v", err)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM cron_jobs WHERE job_id = ?", "job-1").Scan(&status); err != nil {
		t.Fatalf("read cron job status: %v", err)
	}
	if status != "fired" {
		t.Fatalf("status=%q, want=fired", status)
	}
}

func TestNewDBFailsOnSchemaVersionMismatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB initial: %v", err)
	}
	_ = db.Close()

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(`UPDATE metadata SET value = '999', updated_at = datetime('now') WHERE key = 'schema_version'`); err != nil {
		raw.Close()
		t.Fatalf("update schema_version: %v", err)
	}
	_ = raw.Close()

	if _, err := NewDB(dbPath); err == nil {
		t.Fatal("expected schema version mismatch error")
	}
}

func TestListActivePendingDecisions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := db.UpsertPendingDecision("req-a", "EURUSD", `{"action":"HOLD"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision req-a: %v", err)
	}
	if err := db.UpsertPendingDecision("req-b", "XAUUSD", `{"action":"HOLD"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision req-b: %v", err)
	}
	if err := db.ConsumePendingDecision("req-b"); err != nil {
		t.Fatalf("ConsumePendingDecision req-b: %v", err)
	}

	rows, err := db.ListActivePendingDecisions(10)
	if err != nil {
		t.Fatalf("ListActivePendingDecisions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows)=%d, want=1", len(rows))
	}
	if rows[0].RequestID != "req-a" {
		t.Fatalf("request_id=%q, want=req-a", rows[0].RequestID)
	}
}

func TestListDecisionHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := db.UpsertPendingDecision("req-1", "EURUSD", `{"action":"PLACE_PENDING","reason":"breakout"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision req-1: %v", err)
	}
	if err := db.MarkPendingDecisionDelivered("req-1"); err != nil {
		t.Fatalf("MarkPendingDecisionDelivered: %v", err)
	}
	if err := db.ConsumePendingDecision("req-1"); err != nil {
		t.Fatalf("ConsumePendingDecision: %v", err)
	}
	if err := db.UpsertPendingDecision("req-2", "XAUUSD", `{"action":"HOLD","reason":"low confidence"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision req-2: %v", err)
	}

	all, err := db.ListDecisionHistory(20, "")
	if err != nil {
		t.Fatalf("ListDecisionHistory all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all)=%d, want=2", len(all))
	}

	filtered, err := db.ListDecisionHistory(20, "EURUSD")
	if err != nil {
		t.Fatalf("ListDecisionHistory filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("len(filtered)=%d, want=1", len(filtered))
	}
	if filtered[0].RequestID != "req-1" || filtered[0].Decision != "PLACE_PENDING" {
		t.Fatalf("unexpected filtered record: %+v", filtered[0])
	}
}

func TestGetTradeSummary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	winners := []float64{20, 35}
	losers := []float64{-10}
	for i, pnl := range append(winners, losers...) {
		tr := &Trade{
			Symbol:     "EURUSD",
			Direction:  "BUY",
			Entry:      1.10,
			Lot:        0.10,
			SL:         1.09,
			TP:         1.12,
			Session:    "LONDON",
			Timeframe:  "H1",
			LLMReason:  "test",
			Confidence: 60,
			OpenedAt:   now.Add(time.Duration(i) * time.Minute),
		}
		id, err := db.InsertTrade(tr)
		if err != nil {
			t.Fatalf("InsertTrade: %v", err)
		}
		if err := db.CloseTrade(id, 1.11, pnl, now.Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatalf("CloseTrade: %v", err)
		}
	}

	summary, err := db.GetTradeSummary(30)
	if err != nil {
		t.Fatalf("GetTradeSummary: %v", err)
	}
	if summary.TotalTrades != 3 {
		t.Fatalf("total_trades=%d, want=3", summary.TotalTrades)
	}
	if summary.Wins != 2 || summary.Losses != 1 {
		t.Fatalf("wins/losses=%d/%d, want=2/1", summary.Wins, summary.Losses)
	}
	if summary.WinRate <= 0.66 || summary.WinRate >= 0.67 {
		t.Fatalf("win_rate=%f, want ~=0.6667", summary.WinRate)
	}
	if summary.TotalPnL != 45 {
		t.Fatalf("total_pnl=%f, want=45", summary.TotalPnL)
	}
}

func TestGetTradeSummaryExcludesOpenTrades(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	closedID, err := db.InsertTrade(&Trade{
		Symbol:     "EURUSD",
		Direction:  "BUY",
		Entry:      1.10,
		Lot:        0.10,
		SL:         1.09,
		TP:         1.12,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "closed",
		Confidence: 70,
		OpenedAt:   now.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("InsertTrade closed: %v", err)
	}
	if err := db.CloseTrade(closedID, 1.11, 12.0, now.Add(-20*time.Minute)); err != nil {
		t.Fatalf("CloseTrade closed: %v", err)
	}

	if _, err := db.InsertTrade(&Trade{
		Symbol:     "EURUSD",
		Direction:  "BUY",
		Entry:      1.10,
		Lot:        0.10,
		SL:         1.09,
		TP:         1.12,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "open",
		Confidence: 70,
		OpenedAt:   now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertTrade open: %v", err)
	}

	summary, err := db.GetTradeSummary(30)
	if err != nil {
		t.Fatalf("GetTradeSummary: %v", err)
	}
	if summary.TotalTrades != 1 {
		t.Fatalf("total_trades=%d, want=1 (open trades must be excluded)", summary.TotalTrades)
	}
	if summary.TotalPnL != 12.0 {
		t.Fatalf("total_pnl=%f, want=12", summary.TotalPnL)
	}
}

func TestGetEquityCurveUsesClosedTradesOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	firstID, err := db.InsertTrade(&Trade{
		Symbol:     "XAUUSD",
		Direction:  "BUY",
		Entry:      100,
		Lot:        0.10,
		SL:         99,
		TP:         101,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "first",
		Confidence: 60,
		OpenedAt:   now.Add(-3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertTrade first: %v", err)
	}
	if err := db.CloseTrade(firstID, 101, 5.0, now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("CloseTrade first: %v", err)
	}

	if _, err := db.InsertTrade(&Trade{
		Symbol:     "XAUUSD",
		Direction:  "BUY",
		Entry:      100,
		Lot:        0.10,
		SL:         99,
		TP:         101,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "open",
		Confidence: 60,
		OpenedAt:   now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertTrade open: %v", err)
	}

	secondID, err := db.InsertTrade(&Trade{
		Symbol:     "XAUUSD",
		Direction:  "SELL",
		Entry:      100,
		Lot:        0.10,
		SL:         101,
		TP:         99,
		Session:    "NY",
		Timeframe:  "H1",
		LLMReason:  "second",
		Confidence: 60,
		OpenedAt:   now.Add(-70 * time.Minute),
	})
	if err != nil {
		t.Fatalf("InsertTrade second: %v", err)
	}
	if err := db.CloseTrade(secondID, 99.5, -2.0, now.Add(-60*time.Minute)); err != nil {
		t.Fatalf("CloseTrade second: %v", err)
	}

	points, err := db.GetEquityCurve(30)
	if err != nil {
		t.Fatalf("GetEquityCurve: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("len(points)=%d, want=2 (open trades must be excluded)", len(points))
	}
	if points[0].Value != 5.0 {
		t.Fatalf("points[0].value=%f, want=5.0", points[0].Value)
	}
	if points[1].Value != 3.0 {
		t.Fatalf("points[1].value=%f, want=3.0", points[1].Value)
	}
}

func TestGetPairAnalyticsUsesClosedTradesOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	eurClosed, err := db.InsertTrade(&Trade{
		Symbol:     "EURUSD",
		Direction:  "BUY",
		Entry:      1.10,
		Lot:        0.10,
		SL:         1.09,
		TP:         1.12,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "eur closed",
		Confidence: 60,
		OpenedAt:   now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertTrade eur closed: %v", err)
	}
	if err := db.CloseTrade(eurClosed, 1.11, 9.0, now.Add(-90*time.Minute)); err != nil {
		t.Fatalf("CloseTrade eur closed: %v", err)
	}

	if _, err := db.InsertTrade(&Trade{
		Symbol:     "EURUSD",
		Direction:  "BUY",
		Entry:      1.10,
		Lot:        0.10,
		SL:         1.09,
		TP:         1.12,
		Session:    "LONDON",
		Timeframe:  "H1",
		LLMReason:  "eur open",
		Confidence: 60,
		OpenedAt:   now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertTrade eur open: %v", err)
	}

	xauClosed, err := db.InsertTrade(&Trade{
		Symbol:     "XAUUSD",
		Direction:  "SELL",
		Entry:      2000,
		Lot:        0.10,
		SL:         2005,
		TP:         1990,
		Session:    "NY",
		Timeframe:  "H1",
		LLMReason:  "xau closed",
		Confidence: 60,
		OpenedAt:   now.Add(-80 * time.Minute),
	})
	if err != nil {
		t.Fatalf("InsertTrade xau closed: %v", err)
	}
	if err := db.CloseTrade(xauClosed, 2001, -4.0, now.Add(-70*time.Minute)); err != nil {
		t.Fatalf("CloseTrade xau closed: %v", err)
	}

	rows, err := db.GetPairAnalytics(30)
	if err != nil {
		t.Fatalf("GetPairAnalytics: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows)=%d, want=2", len(rows))
	}

	pairs := map[string]PairAnalytics{}
	for _, row := range rows {
		pairs[row.Symbol] = row
	}

	eur := pairs["EURUSD"]
	if eur.Trades != 1 || eur.Wins != 1 || eur.TotalPnL != 9.0 {
		t.Fatalf("EURUSD analytics unexpected: %+v", eur)
	}
	xau := pairs["XAUUSD"]
	if xau.Trades != 1 || xau.Wins != 0 || xau.TotalPnL != -4.0 {
		t.Fatalf("XAUUSD analytics unexpected: %+v", xau)
	}
}
