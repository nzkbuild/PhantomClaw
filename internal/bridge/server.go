package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SignalRequest is the payload sent by MT5 EA on candle close / threshold breach.
type SignalRequest struct {
	Symbol     string             `json:"symbol"`
	Timeframe  string             `json:"timeframe"`
	Bid        float64            `json:"bid"`
	Ask        float64            `json:"ask"`
	Spread     float64            `json:"spread"`
	Balance    float64            `json:"balance,omitempty"`
	Equity     float64            `json:"equity,omitempty"`
	Margin     float64            `json:"margin,omitempty"`
	FreeMargin float64            `json:"free_margin,omitempty"`
	OpenPos    int                `json:"open_positions,omitempty"`
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

// AccountSnapshot holds the latest account state sent by MT5.
type AccountSnapshot struct {
	Balance       float64 `json:"balance"`
	Equity        float64 `json:"equity"`
	Margin        float64 `json:"margin"`
	FreeMargin    float64 `json:"free_margin"`
	OpenPositions int     `json:"open_positions"`
	Timestamp     string  `json:"timestamp"`
}

// Server is the HTTP REST bridge between PhantomClaw and MT5 EA.
type Server struct {
	host          string
	port          int
	server        *http.Server
	onSignal      SignalHandler
	onTradeResult TradeResultHandler

	mu               sync.RWMutex
	latestBySymbol   map[string]SignalRequest
	pendingBySymbol  map[string]SignalResponse
	latestAccount    AccountSnapshot
	hasAccountSample bool
}

// NewServer creates a new bridge server.
func NewServer(host string, port int, onSignal SignalHandler, onTradeResult TradeResultHandler) *Server {
	s := &Server{
		host:            host,
		port:            port,
		onSignal:        onSignal,
		onTradeResult:   onTradeResult,
		latestBySymbol:  make(map[string]SignalRequest),
		pendingBySymbol: make(map[string]SignalResponse),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /signal", s.handleSignal)
	mux.HandleFunc("GET /decision", s.handleDecision)
	mux.HandleFunc("POST /trade-result", s.handleTradeResult)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /price", s.handlePrice)
	mux.HandleFunc("GET /account", s.handleAccount)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 2 * time.Second, // /signal is async and returns immediate ACK
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

	s.mu.Lock()
	s.latestBySymbol[strings.ToUpper(req.Symbol)] = req
	s.latestAccount = AccountSnapshot{
		Balance:       req.Balance,
		Equity:        req.Equity,
		Margin:        req.Margin,
		FreeMargin:    req.FreeMargin,
		OpenPositions: req.OpenPos,
		Timestamp:     req.Timestamp,
	}
	s.hasAccountSample = true
	s.mu.Unlock()

	// Process the signal asynchronously so EA can use a very short timeout.
	if s.onSignal != nil {
		reqCopy := req
		go func(r SignalRequest) {
			decision := s.onSignal(&r)
			if decision == nil {
				decision = &SignalResponse{Action: "HOLD", Reason: "no decision"}
			}

			symbol := strings.ToUpper(strings.TrimSpace(r.Symbol))
			if symbol == "" {
				symbol = strings.ToUpper(strings.TrimSpace(decision.Symbol))
			}
			if symbol == "" {
				return
			}

			s.mu.Lock()
			s.pendingBySymbol[symbol] = *decision
			s.mu.Unlock()
		}(reqCopy)
	}

	// Immediate ACK (fire-and-forget).
	resp := &SignalResponse{
		Action: "HOLD",
		Reason: "accepted_async",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDecision returns the latest pending decision for a symbol, if available.
// Decision is consumed once (removed after read).
func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		http.Error(w, `{"error":"symbol is required"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	decision, ok := s.pendingBySymbol[symbol]
	if ok {
		delete(s.pendingBySymbol, symbol)
	}
	s.mu.Unlock()

	if !ok {
		decision = SignalResponse{
			Action: "HOLD",
			Reason: "no pending decision",
			Symbol: symbol,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(decision)
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

// handlePrice returns the latest MT5 snapshot for a symbol.
func (s *Server) handlePrice(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		http.Error(w, `{"error":"symbol is required"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	snapshot, ok := s.latestBySymbol[symbol]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"no snapshot for symbol yet"}`, http.StatusNotFound)
		return
	}

	out := map[string]any{
		"symbol":    snapshot.Symbol,
		"bid":       snapshot.Bid,
		"ask":       snapshot.Ask,
		"spread":    snapshot.Spread,
		"timestamp": snapshot.Timestamp,
		"source":    "mt5_bridge_snapshot",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleAccount returns the latest MT5 account snapshot.
func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	account := s.latestAccount
	hasSample := s.hasAccountSample
	s.mu.RUnlock()

	if !hasSample {
		http.Error(w, `{"error":"no account snapshot yet"}`, http.StatusNotFound)
		return
	}

	out := map[string]any{
		"balance":        account.Balance,
		"equity":         account.Equity,
		"margin":         account.Margin,
		"free_margin":    account.FreeMargin,
		"open_positions": account.OpenPositions,
		"timestamp":      account.Timestamp,
		"source":         "mt5_bridge_snapshot",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
