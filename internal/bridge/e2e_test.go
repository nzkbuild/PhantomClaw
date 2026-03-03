package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

type fakeBrain struct {
	mu           sync.Mutex
	signalCalls  int
	tradeResults []TradeResultRequest
}

func (f *fakeBrain) HandleSignal(_ context.Context, req *SignalRequest) *SignalResponse {
	f.mu.Lock()
	f.signalCalls++
	f.mu.Unlock()

	return &SignalResponse{
		RequestID: req.RequestID,
		Action:    "PLACE_PENDING",
		Type:      "BUY_LIMIT",
		Symbol:    req.Symbol,
		Level:     req.Bid - 0.0002,
		Lot:       0.10,
		SL:        req.Bid - 0.0010,
		TP:        req.Bid + 0.0020,
		Reason:    "mocked-brain",
	}
}

func (f *fakeBrain) HandleTradeResult(req *TradeResultRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tradeResults = append(f.tradeResults, *req)
}

func (f *fakeBrain) HandleChat(_ context.Context, userText string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprintf("signals=%d closed=%d prompt=%s", f.signalCalls, len(f.tradeResults), strings.TrimSpace(userText)), nil
}

func TestBridgeE2EMT5AndTelegramMock(t *testing.T) {
	SetVersion("e2e-test")
	SetContractVersion("v3")

	dbPath := filepath.Join(t.TempDir(), "bridge-e2e.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	brain := &fakeBrain{}
	s := NewServer("127.0.0.1", 0, brain.HandleSignal, brain.HandleTradeResult, db, "bridge-secret")
	ts := httptest.NewServer(s.server.Handler)
	defer ts.Close()

	signalPayload := map[string]any{
		"request_id":     "req-e2e-1",
		"symbol":         "EURUSD",
		"timeframe":      "H1",
		"bid":            1.1000,
		"ask":            1.1002,
		"spread":         2.0,
		"balance":        10000.0,
		"equity":         9998.0,
		"margin":         120.0,
		"free_margin":    9878.0,
		"open_positions": 1,
		"ohlcv": map[string]any{
			"H1": []map[string]any{},
		},
		"indicators": map[string]any{
			"rsi_14": 48.2,
		},
		"timestamp": "2026-03-03 16:00:00",
	}

	ackBody := doJSONRequest(t, http.MethodPost, ts.URL+"/signal", signalPayload, "bridge-secret", "v3")
	var ack SignalResponse
	if err := json.Unmarshal(ackBody, &ack); err != nil {
		t.Fatalf("decode /signal ack: %v", err)
	}
	if ack.Action != "HOLD" || ack.Reason != "accepted_async" || ack.RequestID != "req-e2e-1" {
		t.Fatalf("unexpected /signal ack: %+v", ack)
	}

	decision := pollDecisionByRequestID(t, ts.URL, "req-e2e-1", "bridge-secret", "v3")
	if decision.Action != "PLACE_PENDING" || decision.Symbol != "EURUSD" || decision.Type != "BUY_LIMIT" {
		t.Fatalf("unexpected decision payload: %+v", decision)
	}

	priceBody := doRequest(t, http.MethodGet, ts.URL+"/price?symbol=EURUSD", nil, "bridge-secret", "v3")
	var pricePayload map[string]any
	if err := json.Unmarshal(priceBody, &pricePayload); err != nil {
		t.Fatalf("decode /price: %v", err)
	}
	if pricePayload["symbol"] != "EURUSD" {
		t.Fatalf("price symbol=%v, want=EURUSD", pricePayload["symbol"])
	}

	accountBody := doRequest(t, http.MethodGet, ts.URL+"/account", nil, "bridge-secret", "v3")
	var accountPayload map[string]any
	if err := json.Unmarshal(accountBody, &accountPayload); err != nil {
		t.Fatalf("decode /account: %v", err)
	}
	if int(accountPayload["open_positions"].(float64)) != 1 {
		t.Fatalf("open_positions=%v, want=1", accountPayload["open_positions"])
	}

	_ = doRequest(t, http.MethodGet, ts.URL+"/decision?request_id=req-e2e-1&consume=1", nil, "bridge-secret", "v3")
	afterConsume := getDecisionHTTP(t, ts.URL, "req-e2e-1", "bridge-secret", "v3")
	if afterConsume.Action != "HOLD" || afterConsume.Reason != "no pending decision" {
		t.Fatalf("expected consumed decision state, got: %+v", afterConsume)
	}

	tradePayload := map[string]any{
		"ticket":    555001,
		"symbol":    "EURUSD",
		"direction": "BUY",
		"entry":     1.1000,
		"exit":      1.1020,
		"lot":       0.10,
		"pnl":       20.0,
		"closed_at": "2026-03-03 17:00:00",
	}
	_ = doJSONRequest(t, http.MethodPost, ts.URL+"/trade-result", tradePayload, "bridge-secret", "v3")

	chatReply, err := brain.HandleChat(context.Background(), "status?")
	if err != nil {
		t.Fatalf("HandleChat: %v", err)
	}
	if !strings.Contains(chatReply, "signals=1") || !strings.Contains(chatReply, "closed=1") {
		t.Fatalf("unexpected chat reply: %q", chatReply)
	}
}

func pollDecisionByRequestID(t *testing.T, baseURL, requestID, token, contract string) SignalResponse {
	t.Helper()
	for i := 0; i < 50; i++ {
		decision := getDecisionHTTP(t, baseURL, requestID, token, contract)
		if decision.Action != "HOLD" || decision.Reason != "no pending decision" {
			return decision
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for decision request_id=%s", requestID)
	return SignalResponse{}
}

func getDecisionHTTP(t *testing.T, baseURL, requestID, token, contract string) SignalResponse {
	t.Helper()
	body := doRequest(t, http.MethodGet, baseURL+"/decision?request_id="+requestID, nil, token, contract)
	var decision SignalResponse
	if err := json.Unmarshal(body, &decision); err != nil {
		t.Fatalf("decode /decision: %v", err)
	}
	return decision
}

func doJSONRequest(t *testing.T, method, url string, payload any, token, contract string) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return doRequest(t, method, url, raw, token, contract)
}

func doRequest(t *testing.T, method, url string, body []byte, token, contract string) []byte {
	t.Helper()
	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader([]byte{})
	} else {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(bridgeAuthHeader, token)
	}
	if contract != "" {
		req.Header.Set(contractVersionHeader, contract)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get(contractVersionHeader); got == "" {
		t.Fatalf("missing %s header", contractVersionHeader)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d for %s %s", resp.StatusCode, method, url)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return out.Bytes()
}
