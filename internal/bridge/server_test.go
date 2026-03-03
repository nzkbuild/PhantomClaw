package bridge

import (
	"context"
	"encoding/json"
	"math"
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
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil, "")

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

	delivered := getDecision(t, s, "req-123", "")
	if delivered.Action != "PLACE_PENDING" || delivered.RequestID != "req-123" {
		t.Fatalf("expected delivered decision on second read, got: %+v", delivered)
	}

	consumeRead := getDecisionWithConsume(t, s, "req-123", "", true)
	if consumeRead.Action != "PLACE_PENDING" || consumeRead.RequestID != "req-123" {
		t.Fatalf("expected decision on consume read, got: %+v", consumeRead)
	}
	consumed := getDecision(t, s, "req-123", "")
	if consumed.Action != "HOLD" || consumed.Reason != "no pending decision" {
		t.Fatalf("unexpected consumed follow-up response: %+v", consumed)
	}
}

func TestBridgeGeneratesRequestIDAndKeepsSymbolCompatibility(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "SELL_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil, "")

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

	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "persisted",
		}
	}, nil, db, "")

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

	stillDelivered := getDecision(t, s, "req-persist", "")
	if stillDelivered.Action != "PLACE_PENDING" || stillDelivered.RequestID != "req-persist" {
		t.Fatalf("expected delivered decision before consume, got: %+v", stillDelivered)
	}

	consumeRead := getDecisionWithConsume(t, s, "req-persist", "", true)
	if consumeRead.Action != "PLACE_PENDING" || consumeRead.RequestID != "req-persist" {
		t.Fatalf("expected decision on consume read, got: %+v", consumeRead)
	}
	consumed := getDecision(t, s, "req-persist", "")
	if consumed.Action != "HOLD" || consumed.Reason != "no pending decision" {
		t.Fatalf("expected consumed state after db consume, got: %+v", consumed)
	}
}

func TestBridgeDecisionConsumeEndpoint(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil, "")

	payload := `{
		"request_id":"req-consume-endpoint",
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

	if _, ok := waitForDecision(t, s, "req-consume-endpoint", ""); !ok {
		t.Fatal("expected decision before consume")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/decision/consume?request_id=req-consume-endpoint", nil)
	s.handleDecisionConsume(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("consume endpoint status=%d body=%s", rec.Code, rec.Body.String())
	}

	after := getDecision(t, s, "req-consume-endpoint", "")
	if after.Action != "HOLD" || after.Reason != "no pending decision" {
		t.Fatalf("expected consumed decision, got: %+v", after)
	}
}

func TestSignalContextTimeout(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		select {
		case <-time.After(500 * time.Millisecond):
			return &SignalResponse{
				Action: "PLACE_PENDING",
				Type:   "BUY_LIMIT",
				Symbol: req.Symbol,
				Reason: "unexpected completion",
			}
		case <-ctx.Done():
			return &SignalResponse{
				Action: "HOLD",
				Symbol: req.Symbol,
				Reason: ctx.Err().Error(),
			}
		}
	}, nil, nil, "")
	s.signalTimeout = 25 * time.Millisecond

	payload := `{
		"request_id":"req-timeout",
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

	decision, ok := waitForDecision(t, s, "req-timeout", "")
	if !ok {
		t.Fatal("expected decision to be stored after timeout cancellation")
	}
	if decision.Action != "HOLD" {
		t.Fatalf("decision action=%q, want=HOLD", decision.Action)
	}
	if !strings.Contains(decision.Reason, context.DeadlineExceeded.Error()) {
		t.Fatalf("decision reason=%q, want contains=%q", decision.Reason, context.DeadlineExceeded.Error())
	}
}

func TestTradeResultIncludesEntry(t *testing.T) {
	got := make(chan TradeResultRequest, 1)
	s := NewServer("127.0.0.1", 0, nil, func(req *TradeResultRequest) {
		got <- *req
	}, nil, "")

	payload := `{
		"ticket":12345,
		"symbol":"EURUSD",
		"direction":"BUY",
		"entry":1.10234,
		"exit":1.10400,
		"lot":0.10,
		"pnl":16.6,
		"closed_at":"2026-03-02 21:00:00"
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/trade-result", strings.NewReader(payload))
	s.handleTradeResult(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trade-result status=%d body=%s", rec.Code, rec.Body.String())
	}

	select {
	case tr := <-got:
		if math.Abs(tr.Entry-1.10234) > 1e-9 {
			t.Fatalf("entry=%f, want=1.10234", tr.Entry)
		}
	default:
		t.Fatal("expected trade-result callback to be invoked")
	}
}

func TestTradeResultRejectsMissingEntry(t *testing.T) {
	called := false
	s := NewServer("127.0.0.1", 0, nil, func(req *TradeResultRequest) {
		called = true
	}, nil, "")

	payload := `{
		"ticket":12345,
		"symbol":"EURUSD",
		"direction":"BUY",
		"exit":1.10400,
		"lot":0.10,
		"pnl":16.6,
		"closed_at":"2026-03-02 21:00:00"
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/trade-result", strings.NewReader(payload))
	s.handleTradeResult(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("trade-result status=%d, want=%d", rec.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("trade-result callback should not be called for invalid payload")
	}
}

func TestBridgeAuthRejectsUnauthorizedRequests(t *testing.T) {
	onSignalCalled := false
	onTradeResultCalled := false
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		onSignalCalled = true
		return &SignalResponse{Action: "HOLD"}
	}, func(req *TradeResultRequest) {
		onTradeResultCalled = true
	}, nil, "bridge-secret")

	signalPayload := `{
		"symbol":"EURUSD",
		"timeframe":"H1",
		"bid":1.10000,
		"ask":1.10020,
		"spread":2.0,
		"ohlcv":{"H1":[]},
		"indicators":{"rsi_14":45.0},
		"timestamp":"2026-03-02 20:00:00"
	}`
	signalRec := httptest.NewRecorder()
	signalReq := httptest.NewRequest(http.MethodPost, "/signal", strings.NewReader(signalPayload))
	s.handleSignal(signalRec, signalReq)
	if signalRec.Code != http.StatusUnauthorized {
		t.Fatalf("signal status=%d, want=%d", signalRec.Code, http.StatusUnauthorized)
	}
	if onSignalCalled {
		t.Fatal("onSignal should not be called for unauthorized requests")
	}

	decisionRec := httptest.NewRecorder()
	decisionReq := httptest.NewRequest(http.MethodGet, "/decision?symbol=EURUSD", nil)
	s.handleDecision(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusUnauthorized {
		t.Fatalf("decision status=%d, want=%d", decisionRec.Code, http.StatusUnauthorized)
	}

	tradePayload := `{
		"ticket":12345,
		"symbol":"EURUSD",
		"direction":"BUY",
		"entry":1.10234,
		"exit":1.10400,
		"lot":0.10,
		"pnl":16.6,
		"closed_at":"2026-03-02 21:00:00"
	}`
	tradeRec := httptest.NewRecorder()
	tradeReq := httptest.NewRequest(http.MethodPost, "/trade-result", strings.NewReader(tradePayload))
	s.handleTradeResult(tradeRec, tradeReq)
	if tradeRec.Code != http.StatusUnauthorized {
		t.Fatalf("trade-result status=%d, want=%d", tradeRec.Code, http.StatusUnauthorized)
	}
	if onTradeResultCalled {
		t.Fatal("onTradeResult should not be called for unauthorized requests")
	}
}

func TestBridgeModelsEndpoint(t *testing.T) {
	s := NewServer("127.0.0.1", 0, nil, nil, nil, "bridge-secret")
	s.SetModelsProvider(func() any {
		return map[string]any{
			"current_provider": "claude",
			"providers": []map[string]any{
				{"provider": "claude", "model": "claude-sonnet-4-20250514", "status": "healthy", "current": true},
				{"provider": "groq", "model": "llama-3.3-70b-versatile", "status": "healthy", "current": false},
			},
			"aliases": map[string]string{"fast": "groq"},
		}
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	req.Header.Set(bridgeAuthHeader, "bridge-secret")
	s.handleModels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d, want=%d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /models: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected status payload: %+v", payload)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got: %#v", payload["data"])
	}
	if data["current_provider"] != "claude" {
		t.Fatalf("current_provider=%v, want=claude", data["current_provider"])
	}
}

func TestBridgeDiagnosticsEndpoint(t *testing.T) {
	s := NewServer("127.0.0.1", 0, nil, nil, nil, "bridge-secret")
	s.SetDiagnosticsProvider(func(ctx context.Context) (map[string]any, error) {
		return map[string]any{
			"bridge": map[string]any{"status": "ok"},
			"db":     map[string]any{"status": "ok"},
		}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/diagnostics", nil)
	req.Header.Set(bridgeAuthHeader, "bridge-secret")
	s.handleDiagnostics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected diagnostics payload: %+v", payload)
	}
}

func TestBridgeAdminDecisionsEndpoint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge-admin.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := db.UpsertPendingDecision("req-admin", "EURUSD", `{"action":"HOLD","reason":"test"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision: %v", err)
	}

	s := NewServer("127.0.0.1", 0, nil, nil, db, "bridge-secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/decisions?limit=10&symbol=EURUSD", nil)
	req.Header.Set(bridgeAuthHeader, "bridge-secret")
	s.handleAdminDecisions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin decisions status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if payload["count"].(float64) < 1 {
		t.Fatalf("expected at least one decision row: %+v", payload)
	}
}

func TestBridgeAdminLogsEndpoint(t *testing.T) {
	s := NewServer("127.0.0.1", 0, nil, nil, nil, "bridge-secret")
	s.SetLogProvider(func(ctx context.Context, query LogQuery) ([]map[string]any, error) {
		return []map[string]any{
			{"ts": time.Now().UTC().Format(time.RFC3339), "level": "info", "msg": "hello"},
		}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/logs?limit=5&level=info", nil)
	req.Header.Set(bridgeAuthHeader, "bridge-secret")
	s.handleAdminLogs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin logs status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if payload["count"].(float64) != 1 {
		t.Fatalf("expected one log row: %+v", payload)
	}
}

func TestBridgeAuthAllowsAuthorizedRequests(t *testing.T) {
	s := NewServer("127.0.0.1", 0, func(ctx context.Context, req *SignalRequest) *SignalResponse {
		return &SignalResponse{
			Action: "PLACE_PENDING",
			Type:   "BUY_LIMIT",
			Symbol: req.Symbol,
			Reason: "ok",
		}
	}, nil, nil, "bridge-secret")

	signalPayload := `{
		"request_id":"req-auth",
		"symbol":"EURUSD",
		"timeframe":"H1",
		"bid":1.10000,
		"ask":1.10020,
		"spread":2.0,
		"ohlcv":{"H1":[]},
		"indicators":{"rsi_14":45.0},
		"timestamp":"2026-03-02 20:00:00"
	}`
	signalRec := httptest.NewRecorder()
	signalReq := httptest.NewRequest(http.MethodPost, "/signal", strings.NewReader(signalPayload))
	signalReq.Header.Set(bridgeAuthHeader, "bridge-secret")
	s.handleSignal(signalRec, signalReq)
	if signalRec.Code != http.StatusOK {
		t.Fatalf("signal status=%d, want=%d", signalRec.Code, http.StatusOK)
	}

	var decision SignalResponse
	for i := 0; i < 60; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/decision?request_id=req-auth", nil)
		req.Header.Set(bridgeAuthHeader, "bridge-secret")
		s.handleDecision(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("decision status=%d, want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
			t.Fatalf("decode decision: %v", err)
		}
		if decision.Action != "HOLD" || decision.Reason != "no pending decision" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if decision.Action != "PLACE_PENDING" {
		t.Fatalf("decision action=%q, want=PLACE_PENDING", decision.Action)
	}
}

func TestHealthUsesConfiguredVersion(t *testing.T) {
	SetVersion("9.9.9-test")
	SetContractVersion("v3.2")
	s := NewServer("127.0.0.1", 0, nil, nil, nil, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	s.handleHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status=%d, want=%d", rec.Code, http.StatusOK)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode health payload: %v", err)
	}
	if payload["version"] != "9.9.9-test" {
		t.Fatalf("version=%q, want=%q", payload["version"], "9.9.9-test")
	}
	if payload["contract"] != "v3.2" {
		t.Fatalf("contract=%q, want=%q", payload["contract"], "v3.2")
	}
}

func TestBridgeRejectsIncompatibleContractVersion(t *testing.T) {
	SetContractVersion("v3")
	s := NewServer("127.0.0.1", 0, nil, nil, nil, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/decision?symbol=EURUSD", nil)
	req.Header.Set(contractVersionHeader, "v2")
	s.handleDecision(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want=%d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminEndpointsExposeJobsAndQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bridge-admin.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	if err := db.UpsertCronJob("job-1", "EURUSD", "recheck", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("UpsertCronJob: %v", err)
	}
	if err := db.UpsertPendingDecision("req-1", "XAUUSD", `{"action":"HOLD"}`, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("UpsertPendingDecision: %v", err)
	}

	s := NewServer("127.0.0.1", 0, nil, nil, db, "")

	jobsRec := httptest.NewRecorder()
	jobsReq := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	s.handleAdminJobs(jobsRec, jobsReq)
	if jobsRec.Code != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsRec.Code, jobsRec.Body.String())
	}
	var jobsPayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(jobsRec.Body.Bytes(), &jobsPayload); err != nil {
		t.Fatalf("decode jobs payload: %v", err)
	}
	if jobsPayload.Count != 1 {
		t.Fatalf("jobs count=%d, want=1", jobsPayload.Count)
	}

	queueRec := httptest.NewRecorder()
	queueReq := httptest.NewRequest(http.MethodGet, "/admin/queue", nil)
	s.handleAdminQueue(queueRec, queueReq)
	if queueRec.Code != http.StatusOK {
		t.Fatalf("queue status=%d body=%s", queueRec.Code, queueRec.Body.String())
	}
	var queuePayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(queueRec.Body.Bytes(), &queuePayload); err != nil {
		t.Fatalf("decode queue payload: %v", err)
	}
	if queuePayload.Count != 1 {
		t.Fatalf("queue count=%d, want=1", queuePayload.Count)
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
	return getDecisionWithConsume(t, s, requestID, symbol, false)
}

func getDecisionWithConsume(t *testing.T, s *Server, requestID, symbol string, consume bool) SignalResponse {
	t.Helper()
	q := url.Values{}
	if requestID != "" {
		q.Set("request_id", requestID)
	}
	if symbol != "" {
		q.Set("symbol", symbol)
	}
	if consume {
		q.Set("consume", "1")
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
