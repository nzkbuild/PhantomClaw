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
