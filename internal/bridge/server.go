package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// SignalRequest is the payload sent by MT5 EA on candle close / threshold breach.
type SignalRequest struct {
	Symbol     string             `json:"symbol"`
	Timeframe  string             `json:"timeframe"`
	Bid        float64            `json:"bid"`
	Ask        float64            `json:"ask"`
	Spread     float64            `json:"spread"`
	OHLCV      map[string][]OHLCV `json:"ohlcv"`      // keyed by TF: {"H1": [...], "H4": [...]}
	Indicators map[string]float64 `json:"indicators"` // e.g. {"rsi_14": 42.5, "atr_14": 0.0035}
	Timestamp  string             `json:"timestamp"`
}

// OHLCV represents a single candle.
type OHLCV struct {
	Open   float64 `json:"o"`
	High   float64 `json:"h"`
	Low    float64 `json:"l"`
	Close  float64 `json:"c"`
	Volume float64 `json:"v"`
	Time   string  `json:"t"`
}

// SignalResponse is sent back to the EA with the bot's decision.
type SignalResponse struct {
	Action string  `json:"action"`         // PLACE_PENDING | MODIFY_PENDING | CANCEL_PENDING | MARKET_CLOSE | HOLD
	Type   string  `json:"type,omitempty"` // BUY_LIMIT | SELL_LIMIT | BUY_STOP | SELL_STOP
	Symbol string  `json:"symbol,omitempty"`
	Level  float64 `json:"level,omitempty"` // entry price for pending order
	Lot    float64 `json:"lot,omitempty"`
	SL     float64 `json:"sl,omitempty"`
	TP     float64 `json:"tp,omitempty"`
	Ticket int64   `json:"ticket,omitempty"` // for modify/cancel operations
	Reason string  `json:"reason,omitempty"`
}

// TradeResultRequest is pushed by EA when a trade closes.
type TradeResultRequest struct {
	Ticket    int64   `json:"ticket"`
	Symbol    string  `json:"symbol"`
	Direction string  `json:"direction"` // BUY | SELL
	Entry     float64 `json:"entry"`
	Exit      float64 `json:"exit"`
	Lot       float64 `json:"lot"`
	PnL       float64 `json:"pnl"`
	ClosedAt  string  `json:"closed_at"`
}

// SignalHandler is the callback invoked when EA sends a signal.
// Returns the bot's trading decision. Wired to agent logic in Phase 3.
type SignalHandler func(req *SignalRequest) *SignalResponse

// TradeResultHandler is the callback invoked when EA reports trade closure.
type TradeResultHandler func(req *TradeResultRequest)

// Server is the HTTP REST bridge between PhantomClaw and MT5 EA.
type Server struct {
	host          string
	port          int
	server        *http.Server
	onSignal      SignalHandler
	onTradeResult TradeResultHandler
}

// NewServer creates a new bridge server.
func NewServer(host string, port int, onSignal SignalHandler, onTradeResult TradeResultHandler) *Server {
	s := &Server{
		host:          host,
		port:          port,
		onSignal:      onSignal,
		onTradeResult: onTradeResult,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /signal", s.handleSignal)
	mux.HandleFunc("POST /trade-result", s.handleTradeResult)
	mux.HandleFunc("GET /health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 500 * time.Millisecond, // EA has 500ms timeout
		IdleTimeout:  30 * time.Second,
	}

	return s
}

// Start begins listening for EA requests. Blocks until stopped.
func (s *Server) Start() error {
	log.Printf("bridge: listening on %s:%d", s.host, s.port)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the bridge server.
func (s *Server) Stop() error {
	return s.server.Close()
}

// handleSignal processes POST /signal from EA.
func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	var req SignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	// Default response if no handler or handler returns nil
	resp := &SignalResponse{Action: "HOLD", Reason: "no decision"}

	if s.onSignal != nil {
		if decision := s.onSignal(&req); decision != nil {
			resp = decision
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleTradeResult processes POST /trade-result from EA.
func (s *Server) handleTradeResult(w http.ResponseWriter, r *http.Request) {
	var req TradeResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	if s.onTradeResult != nil {
		s.onTradeResult(&req)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"received"}`))
}

// handleHealth responds to GET /health for monitoring.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","service":"phantomclaw","version":"0.1.0"}`))
}
