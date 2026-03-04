package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/bridge"
)

func TestDashboardIndexServesHTML(t *testing.T) {
	s := New("127.0.0.1", 8080, Dependencies{}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "PhantomClaw Control Deck") {
		t.Fatalf("unexpected index body: %s", rec.Body.String())
	}
}

func TestDashboardSnapshotEndpoint(t *testing.T) {
	s := New("127.0.0.1", 8080, Dependencies{
		Snapshot: func(ctx context.Context) (map[string]any, error) {
			return map[string]any{
				"mode": "AUTO",
			}, nil
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["mode"] != "AUTO" {
		t.Fatalf("mode=%v, want=AUTO", payload["mode"])
	}
}

func TestDashboardLogsEndpointParsesFilters(t *testing.T) {
	var got bridge.LogQuery
	s := New("127.0.0.1", 8080, Dependencies{
		Logs: func(ctx context.Context, query bridge.LogQuery) (map[string]any, error) {
			got = query
			return map[string]any{
				"count": 0,
				"logs":  []map[string]any{},
			}, nil
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/logs?limit=15&level=warn&component=bridge&contains=timeout&since=2026-03-04T12:00:00Z",
		nil,
	)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got.Limit != 15 || got.Level != "warn" || got.Component != "bridge" || got.Contains != "timeout" {
		t.Fatalf("unexpected query: %+v", got)
	}
	if got.Since.IsZero() || got.Since.UTC().Format(time.RFC3339) != "2026-03-04T12:00:00Z" {
		t.Fatalf("unexpected since: %v", got.Since)
	}
}

func TestDashboardEquityEndpointParsesDays(t *testing.T) {
	gotDays := 0
	s := New("127.0.0.1", 8080, Dependencies{
		Equity: func(ctx context.Context, days int) (map[string]any, error) {
			gotDays = days
			return map[string]any{
				"days":   days,
				"points": []any{},
			}, nil
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/equity?days=7", nil)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotDays != 7 {
		t.Fatalf("days=%d, want=7", gotDays)
	}
}

func TestDashboardAnalyticsEndpointDefaultsDays(t *testing.T) {
	gotDays := 0
	s := New("127.0.0.1", 8080, Dependencies{
		Analytics: func(ctx context.Context, days int) (map[string]any, error) {
			gotDays = days
			return map[string]any{
				"days":    days,
				"summary": map[string]any{},
				"pairs":   []any{},
			}, nil
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotDays != 30 {
		t.Fatalf("days=%d, want default 30", gotDays)
	}
}
