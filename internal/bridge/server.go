package bridge

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

const defaultSignalHandlerTimeout = 10 * time.Second
const bridgeAuthHeader = "X-Phantom-Bridge-Token"
const contractVersionHeader = "X-Phantom-Bridge-Contract"

var serviceVersion = "unknown"
var contractVersion = "v3"

// SetVersion sets the bridge service version shown by /health.
func SetVersion(v string) {
	if strings.TrimSpace(v) == "" {
		return
	}
	serviceVersion = strings.TrimSpace(v)
}

// SetContractVersion sets the bridge contract version exposed via headers.
func SetContractVersion(v string) {
	if strings.TrimSpace(v) == "" {
		return
	}
	contractVersion = strings.TrimSpace(v)
}

// SignalRequest is the payload sent by MT5 EA on candle close / threshold breach.
type SignalRequest struct {
	RequestID  string             `json:"request_id,omitempty"`
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
	RequestID string  `json:"request_id,omitempty"`
	Action    string  `json:"action"`         // PLACE_PENDING | MODIFY_PENDING | CANCEL_PENDING | MARKET_CLOSE | HOLD
	Type      string  `json:"type,omitempty"` // BUY_LIMIT | SELL_LIMIT | BUY_STOP | SELL_STOP
	Symbol    string  `json:"symbol,omitempty"`
	Level     float64 `json:"level,omitempty"` // entry price for pending order
	Lot       float64 `json:"lot,omitempty"`
	SL        float64 `json:"sl,omitempty"`
	TP        float64 `json:"tp,omitempty"`
	Ticket    int64   `json:"ticket,omitempty"` // for modify/cancel operations
	Reason    string  `json:"reason,omitempty"`
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
type SignalHandler func(ctx context.Context, req *SignalRequest) *SignalResponse

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

// LogQuery controls bridge log export filtering.
type LogQuery struct {
	Level     string
	Component string
	Contains  string
	Since     time.Time
	Limit     int
}

// DiagnosticsProvider returns component-level runtime diagnostics.
type DiagnosticsProvider func(ctx context.Context) (map[string]any, error)

// LogProvider returns filtered structured log rows.
type LogProvider func(ctx context.Context, query LogQuery) ([]map[string]any, error)

// Server is the HTTP REST bridge between PhantomClaw and MT5 EA.
type Server struct {
	host           string
	port           int
	server         *http.Server
	onSignal       SignalHandler
	onTradeResult  TradeResultHandler
	db             *memory.DB
	authToken      string
	modelsProvider func() any
	diagnostics    DiagnosticsProvider
	logProvider    LogProvider

	mu               sync.RWMutex
	latestBySymbol   map[string]SignalRequest
	pendingBySymbol  map[string]SignalResponse
	pendingByRequest map[string]SignalResponse
	latestAccount    AccountSnapshot
	hasAccountSample bool
	requestSeq       uint64
	decisionTTL      time.Duration
	signalTimeout    time.Duration
}

// NewServer creates a new bridge server.
func NewServer(host string, port int, onSignal SignalHandler, onTradeResult TradeResultHandler, db *memory.DB, authToken string) *Server {
	s := &Server{
		host:             host,
		port:             port,
		onSignal:         onSignal,
		onTradeResult:    onTradeResult,
		db:               db,
		authToken:        strings.TrimSpace(authToken),
		latestBySymbol:   make(map[string]SignalRequest),
		pendingBySymbol:  make(map[string]SignalResponse),
		pendingByRequest: make(map[string]SignalResponse),
		decisionTTL:      30 * time.Minute,
		signalTimeout:    defaultSignalHandlerTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /signal", s.handleSignal)
	mux.HandleFunc("GET /decision", s.handleDecision)
	mux.HandleFunc("POST /decision/consume", s.handleDecisionConsume)
	mux.HandleFunc("POST /trade-result", s.handleTradeResult)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /health/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("GET /models", s.handleModels)
	mux.HandleFunc("GET /price", s.handlePrice)
	mux.HandleFunc("GET /account", s.handleAccount)
	mux.HandleFunc("GET /admin/decisions", s.handleAdminDecisions)
	mux.HandleFunc("GET /admin/jobs", s.handleAdminJobs)
	mux.HandleFunc("GET /admin/logs", s.handleAdminLogs)
	mux.HandleFunc("GET /admin/queue", s.handleAdminQueue)

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

// SetSignalTimeout overrides the default signal processing timeout.
func (s *Server) SetSignalTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signalTimeout = timeout
}

// SetAuthToken updates bridge auth token at runtime.
func (s *Server) SetAuthToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authToken = strings.TrimSpace(token)
}

// SetModelsProvider wires a callback for GET /models payload.
func (s *Server) SetModelsProvider(provider func() any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelsProvider = provider
}

// SetDiagnosticsProvider wires callback for rich component diagnostics.
func (s *Server) SetDiagnosticsProvider(provider DiagnosticsProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics = provider
}

// SetLogProvider wires callback for structured log export.
func (s *Server) SetLogProvider(provider LogProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logProvider = provider
}

// handleSignal processes POST /signal from EA.
func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
	var req SignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.RequestID == "" {
		req.RequestID = s.nextRequestID(strings.ToUpper(strings.TrimSpace(req.Symbol)))
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
		go func(r SignalRequest, parent context.Context) {
			ctx, cancel := s.makeSignalContext(parent)
			defer cancel()

			decision := s.onSignal(ctx, &r)
			if decision == nil {
				decision = &SignalResponse{Action: "HOLD", Reason: "no decision", RequestID: r.RequestID}
			}
			if strings.TrimSpace(decision.RequestID) == "" {
				decision.RequestID = r.RequestID
			}

			symbol := strings.ToUpper(strings.TrimSpace(r.Symbol))
			if symbol == "" {
				symbol = strings.ToUpper(strings.TrimSpace(decision.Symbol))
			}
			if decision.Symbol == "" {
				decision.Symbol = symbol
			}

			s.storePendingDecision(*decision, symbol)
		}(reqCopy, r.Context())
	}

	// Immediate ACK (fire-and-forget).
	resp := &SignalResponse{
		RequestID: req.RequestID,
		Action:    "HOLD",
		Reason:    "accepted_async",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDecision returns the latest pending/delivered decision by request_id (preferred) or symbol.
// Read marks pending -> delivered. Consumption is explicit via consume query param or /decision/consume.
func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	consume, consumeExplicit := parseBoolQuery(r.URL.Query().Get("consume"))
	if !consumeExplicit && requestID == "" && symbol != "" {
		// Backward compatibility: legacy symbol-only polling remains one-shot/consuming.
		consume = true
	}
	if s.db != nil {
		_ = s.db.ExpirePendingDecisions(time.Now())
	}
	if requestID == "" && symbol == "" {
		http.Error(w, `{"error":"request_id or symbol is required"}`, http.StatusBadRequest)
		return
	}

	var (
		decision SignalResponse
		ok       bool
		status   string
	)

	if s.db != nil {
		// Prefer request_id correlation if provided.
		if requestID != "" {
			if persisted, dbStatus, found, err := s.db.GetPendingDecisionByRequestID(requestID); err == nil && found {
				if err := json.Unmarshal([]byte(persisted), &decision); err == nil {
					ok = true
					status = dbStatus
					if decision.RequestID == "" {
						decision.RequestID = requestID
					}
				}
			}
		}

		// Fallback to symbol polling for backward compatibility.
		if !ok && symbol != "" {
			if reqID, persisted, dbStatus, found, err := s.db.GetPendingDecisionBySymbol(symbol); err == nil && found {
				if err := json.Unmarshal([]byte(persisted), &decision); err == nil {
					ok = true
					status = dbStatus
					if decision.RequestID == "" {
						decision.RequestID = reqID
					}
				}
			}
		}

		if ok {
			s.storePendingDecisionInMemory(decision, strings.ToUpper(strings.TrimSpace(decision.Symbol)))
			if decision.RequestID != "" && status == "pending" {
				_ = s.db.MarkPendingDecisionDelivered(decision.RequestID)
			}
			if decision.RequestID != "" && consume {
				_ = s.db.ConsumePendingDecision(decision.RequestID)
				s.removePendingDecisionInMemory(decision.RequestID, decision.Symbol)
			}
		}
	} else {
		// In-memory fallback path (if DB unavailable).
		if requestID != "" {
			s.mu.RLock()
			decision, ok = s.pendingByRequest[requestID]
			s.mu.RUnlock()
		}
		if !ok && symbol != "" {
			s.mu.RLock()
			decision, ok = s.pendingBySymbol[symbol]
			s.mu.RUnlock()
		}
		if ok {
			if decision.RequestID != "" && consume {
				s.removePendingDecisionInMemory(decision.RequestID, decision.Symbol)
			}
		}
	}

	if !ok {
		decision = SignalResponse{
			RequestID: requestID,
			Action:    "HOLD",
			Reason:    "no pending decision",
			Symbol:    symbol,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(decision)
}

// handleDecisionConsume explicitly consumes a delivered/pending decision by request_id.
func (s *Server) handleDecisionConsume(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	if requestID == "" {
		var body struct {
			RequestID string `json:"request_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requestID = strings.TrimSpace(body.RequestID)
	}
	if requestID == "" {
		http.Error(w, `{"error":"request_id is required"}`, http.StatusBadRequest)
		return
	}

	if s.db != nil {
		_ = s.db.ConsumePendingDecision(requestID)
	}
	// Best-effort cleanup from in-memory mirrors as well.
	s.mu.RLock()
	decision, ok := s.pendingByRequest[requestID]
	s.mu.RUnlock()
	if ok {
		s.removePendingDecisionInMemory(requestID, decision.Symbol)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "consumed",
		"request_id": requestID,
	})
}

func (s *Server) storePendingDecisionInMemory(decision SignalResponse, symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if symbol != "" {
		s.pendingBySymbol[symbol] = decision
	}
	if decision.RequestID != "" {
		s.pendingByRequest[decision.RequestID] = decision
	}
}

func (s *Server) removePendingDecisionInMemory(requestID, symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requestID != "" {
		delete(s.pendingByRequest, requestID)
	}
	if symbol != "" {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))
		if normalized != "" {
			if current, exists := s.pendingBySymbol[normalized]; exists {
				if requestID == "" || current.RequestID == requestID {
					delete(s.pendingBySymbol, normalized)
				}
			}
		}
	}
}

func parseBoolQuery(v string) (bool, bool) {
	if v == "" {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y":
		return true, true
	case "0", "false", "no", "n":
		return false, true
	default:
		return false, false
	}
}

func (s *Server) storePendingDecision(decision SignalResponse, symbol string) {
	s.storePendingDecisionInMemory(decision, symbol)

	if s.db == nil || decision.RequestID == "" {
		return
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return
	}
	_ = s.db.UpsertPendingDecision(
		decision.RequestID,
		decision.Symbol,
		string(payload),
		time.Now().Add(s.decisionTTL),
	)
}

func (s *Server) makeSignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}

	timeout := s.signalTimeout
	if timeout <= 0 {
		return context.WithCancel(base)
	}

	if parent != nil {
		if deadline, ok := parent.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining > 0 && remaining < timeout {
				timeout = remaining
			}
		}
	}

	return context.WithTimeout(base, timeout)
}

func (s *Server) nextRequestID(symbol string) string {
	seq := atomic.AddUint64(&s.requestSeq, 1)
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	return fmt.Sprintf("%s-%d-%d", symbol, time.Now().UnixNano(), seq)
}

// handleTradeResult processes POST /trade-result from EA.
func (s *Server) handleTradeResult(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
	var req TradeResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}
	if req.Entry <= 0 {
		http.Error(w, `{"error":"entry is required and must be > 0"}`, http.StatusBadRequest)
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
	s.setProtocolHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	out := map[string]string{
		"status":   "ok",
		"service":  "phantomclaw",
		"version":  serviceVersion,
		"contract": contractVersion,
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleDiagnostics returns rich component-level diagnostics.
func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}

	out := map[string]any{
		"status":   "ok",
		"service":  "phantomclaw",
		"version":  serviceVersion,
		"contract": contractVersion,
		"ts":       time.Now().UTC(),
	}

	s.mu.RLock()
	provider := s.diagnostics
	s.mu.RUnlock()

	if provider != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		components, err := provider(ctx)
		if err != nil {
			out["components_error"] = err.Error()
		} else {
			out["components"] = components
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleModels returns runtime model/provider inventory.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}

	s.mu.RLock()
	provider := s.modelsProvider
	s.mu.RUnlock()

	out := map[string]any{
		"status": "ok",
	}
	if provider != nil {
		out["data"] = provider()
	} else {
		out["data"] = map[string]any{
			"current_provider": "",
			"providers":        []any{},
			"aliases":          map[string]string{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handlePrice returns the latest MT5 snapshot for a symbol.
func (s *Server) handlePrice(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
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
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}
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

func (s *Server) handleAdminDecisions(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"memory db unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 200, 2000)
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))

	decisions, err := s.db.ListDecisionHistory(limit, symbol)
	if err != nil {
		http.Error(w, `{"error":"failed to list decisions"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count":     len(decisions),
		"symbol":    symbol,
		"decisions": decisions,
	})
}

func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if !s.requireCompatibleContract(w, r) {
		return
	}

	s.mu.RLock()
	provider := s.logProvider
	s.mu.RUnlock()
	if provider == nil {
		http.Error(w, `{"error":"log provider unavailable"}`, http.StatusNotImplemented)
		return
	}

	query := LogQuery{
		Level:     strings.ToLower(strings.TrimSpace(r.URL.Query().Get("level"))),
		Component: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("component"))),
		Contains:  strings.TrimSpace(r.URL.Query().Get("contains")),
		Limit:     parsePositiveInt(r.URL.Query().Get("limit"), 200, 5000),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			query.Since = parsed
		}
	}

	rows, err := provider(r.Context(), query)
	if err != nil {
		http.Error(w, `{"error":"failed to read logs"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count": len(rows),
		"logs":  rows,
	})
}

func (s *Server) handleAdminJobs(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"memory db unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	jobs, err := s.db.ListPendingCronJobs()
	if err != nil {
		http.Error(w, `{"error":"failed to list jobs"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count": len(jobs),
		"jobs":  jobs,
	})
}

func (s *Server) handleAdminQueue(w http.ResponseWriter, r *http.Request) {
	s.setProtocolHeaders(w)
	if !s.requireAuth(w, r) {
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"memory db unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	entries, err := s.db.ListActivePendingDecisions(100)
	if err != nil {
		http.Error(w, `{"error":"failed to list queue"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count": len(entries),
		"queue": entries,
	})
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	s.mu.RLock()
	tokenExpected := s.authToken
	s.mu.RUnlock()

	if tokenExpected == "" {
		return true
	}

	token := strings.TrimSpace(r.Header.Get(bridgeAuthHeader))
	if subtle.ConstantTimeCompare([]byte(token), []byte(tokenExpected)) == 1 {
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	return false
}

func (s *Server) setProtocolHeaders(w http.ResponseWriter) {
	w.Header().Set(contractVersionHeader, contractVersion)
}

func (s *Server) requireCompatibleContract(w http.ResponseWriter, r *http.Request) bool {
	requested := strings.TrimSpace(r.Header.Get(contractVersionHeader))
	if requested == "" {
		return true // backward compatible if caller doesn't send a contract header
	}

	if contractMajor(requested) != contractMajor(contractVersion) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":            "incompatible contract version",
			"server_contract":  contractVersion,
			"request_contract": requested,
		})
		return false
	}
	return true
}

func contractMajor(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "v") {
		v = v[1:]
	}
	parts := strings.SplitN(v, ".", 2)
	return parts[0]
}

func parsePositiveInt(raw string, fallback int, max int) int {
	value := fallback
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
		value = parsed
	}
	if max > 0 && value > max {
		return max
	}
	return value
}
