package memory

import (
	"path/filepath"
	"testing"
	"time"
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
