package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/bridge"
)

//go:embed assets/index.html
var embeddedAssets embed.FS

// Dependencies provide dashboard API data.
type Dependencies struct {
	Snapshot    func(ctx context.Context) (map[string]any, error)
	Decisions   func(ctx context.Context, limit int, symbol string) (map[string]any, error)
	Sessions    func(ctx context.Context, limit int, pair string) (map[string]any, error)
	Diagnostics func(ctx context.Context) (map[string]any, error)
	Ops         func(ctx context.Context) (map[string]any, error)
	Logs        func(ctx context.Context, query bridge.LogQuery) (map[string]any, error)
	Equity      func(ctx context.Context, days int) (map[string]any, error)
	Analytics   func(ctx context.Context, days int) (map[string]any, error)
	// SwitchModel requests a primary provider switch by name (e.g. "gemini-flash").
	// Returns an error if the name is unknown or the switch is rejected.
	SwitchModel func(ctx context.Context, name string) error
}

// Server hosts dashboard UI and API.
type Server struct {
	host string
	port int
	deps Dependencies

	mux    *http.ServeMux
	server *http.Server
}

// New creates a dashboard server.
// authMiddleware wraps all routes with auth; pass nil to skip auth.
func New(host string, port int, deps Dependencies, authMiddleware func(http.Handler) http.Handler) *Server {
	s := &Server{
		host: host,
		port: port,
		deps: deps,
		mux:  http.NewServeMux(),
	}
	s.registerRoutes()

	var handler http.Handler = s.mux
	if authMiddleware != nil {
		handler = authMiddleware(handler)
	}

	s.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           handler,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		// WriteTimeout is 0 so long-lived SSE connections are not forcibly closed.
		// Each handler manages its own context deadline.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start launches the dashboard HTTP server.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully stops the dashboard server.
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Handler exposes handler for tests.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("GET /api/decisions", s.handleDecisions)
	s.mux.HandleFunc("GET /api/sessions", s.handleSessions)
	s.mux.HandleFunc("GET /api/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("GET /api/ops", s.handleOps)
	s.mux.HandleFunc("GET /api/logs", s.handleLogs)
	s.mux.HandleFunc("GET /api/equity", s.handleEquity)
	s.mux.HandleFunc("GET /api/analytics", s.handleAnalytics)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("POST /api/switch-model", s.handleSwitchModel)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := embeddedAssets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "dashboard asset missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	s.respondWithProvider(w, r, s.deps.Snapshot)
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if s.deps.Decisions == nil {
		http.Error(w, `{"error":"decisions provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 200, 2000)
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Decisions(ctx, limit, symbol)
	if err != nil {
		http.Error(w, `{"error":"failed to load decisions"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.deps.Sessions == nil {
		http.Error(w, `{"error":"sessions provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100, 1000)
	pair := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("pair")))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Sessions(ctx, limit, pair)
	if err != nil {
		http.Error(w, `{"error":"failed to load sessions"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.deps.Diagnostics == nil {
		http.Error(w, `{"error":"diagnostics unavailable"}`, http.StatusNotImplemented)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Diagnostics(ctx)
	if err != nil {
		http.Error(w, `{"error":"diagnostics failed"}`, http.StatusInternalServerError)
		return
	}
	// Redact sensitive fields before sending to browser.
	RedactMap(data)
	writeJSON(w, data)
}

func (s *Server) handleOps(w http.ResponseWriter, r *http.Request) {
	if s.deps.Ops == nil {
		http.Error(w, `{"error":"ops status unavailable"}`, http.StatusNotImplemented)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Ops(ctx)
	if err != nil {
		http.Error(w, `{"error":"ops status failed"}`, http.StatusInternalServerError)
		return
	}
	RedactMap(data)
	writeJSON(w, data)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.deps.Logs == nil {
		http.Error(w, `{"error":"logs provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	query := bridge.LogQuery{
		Level:     strings.ToLower(strings.TrimSpace(r.URL.Query().Get("level"))),
		Component: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("component"))),
		Contains:  strings.TrimSpace(r.URL.Query().Get("contains")),
		Limit:     parsePositiveInt(r.URL.Query().Get("limit"), 200, 5000),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if ts, ok := parseLogTime(raw); ok {
			query.Since = ts
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Logs(ctx, query)
	if err != nil {
		http.Error(w, `{"error":"failed to load logs"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleEquity(w http.ResponseWriter, r *http.Request) {
	if s.deps.Equity == nil {
		http.Error(w, `{"error":"equity provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	days := parsePositiveInt(r.URL.Query().Get("days"), 30, 365)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Equity(ctx, days)
	if err != nil {
		http.Error(w, `{"error":"failed to load equity curve"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.deps.Analytics == nil {
		http.Error(w, `{"error":"analytics provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	days := parsePositiveInt(r.URL.Query().Get("days"), 30, 365)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := s.deps.Analytics(ctx, days)
	if err != nil {
		http.Error(w, `{"error":"failed to load analytics"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

// handleEvents streams snapshot + new log lines as Server-Sent Events.
// The client connects once and receives pushes every 3 s instead of polling.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// sendEvent writes a named SSE event with JSON payload.
	sendEvent := func(name string, payload any) bool {
		b, err := json.Marshal(payload)
		if err != nil {
			return true // skip malformed data
		}
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		if err != nil {
			return false // client disconnected
		}
		flusher.Flush()
		return true
	}

	// Send an initial "connected" ping so the client knows streaming works.
	if !sendEvent("ping", map[string]string{"status": "connected"}) {
		return
	}

	levelFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("level")))
	componentFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("component")))
	containsFilter := strings.TrimSpace(r.URL.Query().Get("contains"))
	var lastLogTs time.Time
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Push snapshot.
			if s.deps.Snapshot != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				snap, err := s.deps.Snapshot(ctx)
				cancel()
				if err == nil {
					if !sendEvent("snapshot", snap) {
						return
					}
				}
			}

			// Push new log lines since last push.
			if s.deps.Logs != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				q := bridge.LogQuery{
					Level:     levelFilter,
					Component: componentFilter,
					Contains:  containsFilter,
					Limit:     50,
					Since:     lastLogTs,
				}
				logData, err := s.deps.Logs(ctx, q)
				cancel()
				if err == nil {
					if !sendEvent("logs", logData) {
						return
					}
					// Advance the log cursor.
					if ts, ok := latestLogTS(logData["logs"]); ok {
						lastLogTs = ts
					}
				}
			}

			// Push operational truth state.
			if s.deps.Ops != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				opsData, err := s.deps.Ops(ctx)
				cancel()
				if err == nil {
					RedactMap(opsData)
					if !sendEvent("ops", opsData) {
						return
					}
				}
			}
		}
	}
}

// handleSwitchModel accepts POST /api/switch-model?name=<provider-name> or JSON body.
func (s *Server) handleSwitchModel(w http.ResponseWriter, r *http.Request) {
	if s.deps.SwitchModel == nil {
		http.Error(w, `{"error":"model switch not available"}`, http.StatusNotImplemented)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			name = strings.TrimSpace(body.Name)
		}
	}
	if name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.deps.SwitchModel(ctx, name); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"status": "queued", "name": name})
}

func (s *Server) respondWithProvider(w http.ResponseWriter, r *http.Request, provider func(context.Context) (map[string]any, error)) {
	if provider == nil {
		http.Error(w, `{"error":"provider unavailable"}`, http.StatusNotImplemented)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := provider(ctx)
	if err != nil {
		http.Error(w, `{"error":"provider failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, data)
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
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

func parseLogTime(raw string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z0700",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05-07:00",
	}
	raw = strings.TrimSpace(raw)
	tryParse := func(candidate string) (time.Time, bool) {
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, candidate); err == nil {
				return ts, true
			}
		}
		return time.Time{}, false
	}
	if ts, ok := tryParse(raw); ok {
		return ts, true
	}

	// URL query decoding treats '+' as space; normalize e.g. "...000 0800".
	if parts := strings.Fields(raw); len(parts) == 2 && strings.Contains(parts[0], "T") {
		offset := parts[1]
		if !strings.HasPrefix(offset, "+") && !strings.HasPrefix(offset, "-") && !strings.HasPrefix(offset, "Z") {
			offset = "+" + offset
		}
		if ts, ok := tryParse(parts[0] + offset); ok {
			return ts, true
		}
	}
	return time.Time{}, false
}

func latestLogTS(logs any) (time.Time, bool) {
	switch rows := logs.(type) {
	case []map[string]any:
		for i := len(rows) - 1; i >= 0; i-- {
			if tsRaw, ok := rows[i]["ts"].(string); ok {
				if ts, ok := parseLogTime(tsRaw); ok {
					return ts, true
				}
			}
		}
	case []any:
		for i := len(rows) - 1; i >= 0; i-- {
			entry, ok := rows[i].(map[string]any)
			if !ok {
				continue
			}
			if tsRaw, ok := entry["ts"].(string); ok {
				if ts, ok := parseLogTime(tsRaw); ok {
					return ts, true
				}
			}
		}
	}
	return time.Time{}, false
}
