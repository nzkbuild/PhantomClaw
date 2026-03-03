package market

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseNewsCacheRoundTrip(t *testing.T) {
	items := []NewsItem{
		{
			Title:    "USD CPI",
			Impact:   "high",
			Currency: "USD",
			Time:     time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC),
			Source:   "forexfactory",
		},
	}

	payload := `[{"title":"USD CPI","impact":"high","currency":"USD","time":"2026-03-03T12:00:00Z","source":"forexfactory"}]`
	parsed, err := parseNewsCache(payload)
	if err != nil {
		t.Fatalf("parseNewsCache: %v", err)
	}
	if len(parsed) != len(items) {
		t.Fatalf("len(parsed)=%d, want=%d", len(parsed), len(items))
	}
	if parsed[0].Title != items[0].Title || parsed[0].Impact != items[0].Impact {
		t.Fatalf("unexpected parsed item: %+v", parsed[0])
	}
}

func TestParseCOTCacheRoundTrip(t *testing.T) {
	payload := `{"symbol":"XAUUSD","commercial_long":10,"commercial_short":2,"commercial_net":8,"large_spec_long":9,"large_spec_short":3,"large_spec_net":6,"small_spec_long":0,"small_spec_short":0,"small_spec_net":0,"report_date":"2026-03-01","net_positioning":"bullish"}`
	parsed, err := parseCOTCache(payload)
	if err != nil {
		t.Fatalf("parseCOTCache: %v", err)
	}
	if parsed.Symbol != "XAUUSD" || parsed.CommNet != 8 || parsed.NetPositioning != "bullish" {
		t.Fatalf("unexpected parsed COT data: %+v", parsed)
	}
}

func TestNewsFailPolicyOpenVsClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "market.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	makeFailingClient := func() *http.Client {
		return &http.Client{
			Timeout: 50 * time.Millisecond,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network down")
			}),
		}
	}

	openFetcher := NewNewsFetcher(db, "fail_open", nil, nil)
	openFetcher.client = makeFailingClient()
	if openFetcher.HasHighImpactEvent("USD") {
		t.Fatal("expected fail_open policy to return false on fetch failure")
	}

	closedFetcher := NewNewsFetcher(db, "fail_closed", nil, nil)
	closedFetcher.client = makeFailingClient()
	if !closedFetcher.HasHighImpactEvent("USD") {
		t.Fatal("expected fail_closed policy to return true on fetch failure")
	}
}
