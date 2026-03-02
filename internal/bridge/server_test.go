package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

func TestBridgeRequestCorrelation(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil)

	payload := `{
		"request_id":"req-123",
		"symbol":"EURUSD",
		"timeframe":"H1",
		"bid":1.10000,
		"ask":1.10020,
		"spread":2.0,
		"ohlcv":{"H1":[]},
		"indicators":{"rsi_14":45.0},
		"timestamp":"2026-03-02 20:00:00"
	}`

	sigRec := httptest.NewRecorder()
	sigReq := httptest.NewRequest(http.MethodPost, "/signal", strings.NewReader(payload))
	s.handleSignal(sigRec, sigReq)
	if sigRec.Code != http.StatusOK {
		t.Fatalf("signal status=%d, want=%d", sigRec.Code, http.StatusOK)
	}

	var ack SignalResponse
	if err := json.Unmarshal(sigRec.Body.Bytes(), &ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ack.Action != "HOLD" || ack.Reason != "accepted_async" {
		t.Fatalf("unexpected ack: %+v", ack)
	}
	if ack.RequestID != "req-123" {
		t.Fatalf("ack request_id=%q, want=req-123", ack.RequestID)
	}

	decision, ok := waitForDecision(t, s, "req-123", "")
	if !ok {
		t.Fatalf("expected pending decision for request_id req-123")
	}
	if decision.Action != "PLACE_PENDING" {
		t.Fatalf("decision action=%q, want=PLACE_PENDING", decision.Action)
	}
	if decision.RequestID != "req-123" {
		t.Fatalf("decision request_id=%q, want=req-123", decision.RequestID)
	}

	consumed := getDecision(t, s, "req-123", "")
	if consumed.Action != "HOLD" || consumed.Reason != "no pending decision" {
		t.Fatalf("unexpected consumed response: %+v", consumed)
	}
}

func TestBridgeGeneratesRequestIDAndKeepsSymbolCompatibility(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "SELL_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil)

	payload := `{
		"symbol":"XAUUSD",
		"timeframe":"H1",
		"bid":2900.10,
		"ask":2900.30,
		"spread":20.0,
		"ohlcv":{"H1":[]},
		"indicators":{"rsi_14":55.0},
		"timestamp":"2026-03-02 20:00:00"
	}`

	sigRec := httptest.NewRecorder()
	sigReq := httptest.NewRequest(http.MethodPost, "/signal", strings.NewReader(payload))
	s.handleSignal(sigRec, sigReq)
	if sigRec.Code != http.StatusOK {
		t.Fatalf("signal status=%d, want=%d", sigRec.Code, http.StatusOK)
	}

	var ack SignalResponse
	if err := json.Unmarshal(sigRec.Body.Bytes(), &ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ack.RequestID == "" {
		t.Fatal("expected generated request_id in ack")
	}

	decision, ok := waitForDecision(t, s, "", "XAUUSD")
	if !ok {
		t.Fatalf("expected pending decision for symbol XAUUSD")
	}
	if decision.Action != "PLACE_PENDING" {
		t.Fatalf("decision action=%q, want=PLACE_PENDING", decision.Action)
	}
	if decision.RequestID != ack.RequestID {
		t.Fatalf("decision request_id=%q, want ack request_id=%q", decision.RequestID, ack.RequestID)
	}

	consumed := getDecision(t, s, ack.RequestID, "")
	if consumed.Action != "HOLD" || consumed.Reason != "no pending decision" {
		t.Fatalf("unexpected consumed response by request_id: %+v", consumed)
	}
}

func TestBridgeDecisionPersistsInSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	s := NewServer("127.0.0.1", 0, func(req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "persisted",
		}
	}, nil, db)

	payload := `{
		"request_id":"req-persist",
		"symbol":"GBPUSD",
		"timeframe":"H1",
		"bid":1.27000,
		"ask":1.27020,
		"spread":2.0,
		"ohlcv":{"H1":[]},
		"indicators":{"rsi_14":48.0},
		"timestamp":"2026-03-02 20:00:00"
	}`
	sigRec := httptest.NewRecorder()
	sigReq := httptest.NewRequest(http.MethodPost, "/signal", strings.NewReader(payload))
	s.handleSignal(sigRec, sigReq)
	if sigRec.Code != http.StatusOK {
		t.Fatalf("signal status=%d, want=%d", sigRec.Code, http.StatusOK)
	}

	decision, ok := waitForDecision(t, s, "req-persist", "")
	if !ok {
		t.Fatal("expected in-memory decision before persistence test")
	}
	if decision.Action != "PLACE_PENDING" {
		t.Fatalf("decision action=%q, want=PLACE_PENDING", decision.Action)
	}

	// Reinsert pending decision and clear in-memory queues to simulate restart.
	err = db.UpsertPendingDecision("req-persist", "GBPUSD", `{"request_id":"req-persist","action":"PLACE_PENDING","type":"BUY_LIMIT","symbol":"GBPUSD","reason":"persisted"}`, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("UpsertPendingDecision: %v", err)
	}
	s.mu.Lock()
	s.pendingByRequest = map[string]SignalResponse{}
	s.pendingBySymbol = map[string]SignalResponse{}
	s.mu.Unlock()

	fromDB := getDecision(t, s, "req-persist", "")
	if fromDB.Action != "PLACE_PENDING" || fromDB.RequestID != "req-persist" {
		t.Fatalf("unexpected db-backed decision: %+v", fromDB)
	}

	consumed := getDecision(t, s, "req-persist", "")
	if consumed.Action != "HOLD" || consumed.Reason != "no pending decision" {
		t.Fatalf("expected consumed state after db delivery, got: %+v", consumed)
	}
}

func waitForDecision(t *testing.T, s *Server, requestID, symbol string) (SignalResponse, bool) {
	t.Helper()
	for i := 0; i < 60; i++ {
		decision := getDecision(t, s, requestID, symbol)
		if decision.Action != "HOLD" || decision.Reason != "no pending decision" {
			return decision, true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return SignalResponse{}, false
}

func getDecision(t *testing.T, s *Server, requestID, symbol string) SignalResponse {
	t.Helper()

	q := url.Values{}
	if requestID != "" {
		q.Set("request_id", requestID)
	}
	if symbol != "" {
		q.Set("symbol", symbol)
	}

	target := "/decision"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	s.handleDecision(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("decision status=%d body=%s", rec.Code, rec.Body.String())
	}

	var decision SignalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	return decision
}
