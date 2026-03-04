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
	Logs        func(ctx context.Context, query bridge.LogQuery) (map[string]any, error)
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
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
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
	s.mux.HandleFunc("GET /api/logs", s.handleLogs)
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
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
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
