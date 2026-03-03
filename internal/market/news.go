package market

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

// NewsItem represents a single news/event item.
type NewsItem struct {
	Title    string    `json:"title"`
	Impact   string    `json:"impact"` // "high" | "medium" | "low"
	Currency string    `json:"currency"`
	Time     time.Time `json:"time"`
	Source   string    `json:"source"`
}

// NewsFetcher retrieves economic news from ForexFactory calendar (RSS).
type NewsFetcher struct {
	db         *memory.DB
	client     *http.Client
	failPolicy FailPolicy
}

// FailPolicy controls how fetch/parsing failures are handled in safety checks.
type FailPolicy string

const (
	FailPolicyOpen   FailPolicy = "fail_open"
	FailPolicyClosed FailPolicy = "fail_closed"
)

// NewNewsFetcher creates a news fetcher with cache support.
func NewNewsFetcher(db *memory.DB, failPolicy string) *NewsFetcher {
	return &NewsFetcher{
		db:         db,
		client:     &http.Client{Timeout: 10 * time.Second},
		failPolicy: normalizeFailPolicy(failPolicy),
	}
}

// forexFactoryRSS is the RSS feed for ForexFactory calendar.
type forexFactoryRSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

// FetchNews retrieves today's economic events and caches them.
func (nf *NewsFetcher) FetchNews() ([]NewsItem, error) {
	// Check cache first (15-min TTL)
	cached, found, err := nf.db.GetCache("news_today")
	if err == nil && found {
		items, parseErr := parseNewsCache(cached)
		if parseErr == nil {
			return items, nil
		}
	}

	// Fetch from ForexFactory RSS
	resp, err := nf.client.Get("https://www.forexfactory.com/rss")
	if err != nil {
		return nil, fmt.Errorf("news: fetch error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rss forexFactoryRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("news: parse error: %w", err)
	}

	var items []NewsItem
	for _, item := range rss.Channel.Items {
		impact := classifyImpact(item.Title)
		currency := extractCurrency(item.Title)
		pubDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
		items = append(items, NewsItem{
			Title:    item.Title,
			Impact:   impact,
			Currency: currency,
			Time:     pubDate,
			Source:   "forexfactory",
		})
	}

	// Cache for 15 minutes
	if payload, marshalErr := json.Marshal(items); marshalErr == nil {
		_ = nf.db.SetCache("news_today", string(payload), "forexfactory",
			time.Now().Add(15*time.Minute))
	}

	return items, nil
}

// HasHighImpactEvent checks if there's a high-impact event for the given currency.
func (nf *NewsFetcher) HasHighImpactEvent(currency string) bool {
	items, err := nf.FetchNews()
	if err != nil {
		return nf.failPolicy == FailPolicyClosed
	}
	for _, item := range items {
		if item.Impact == "high" && strings.Contains(item.Currency, currency) {
			return true
		}
	}
	return false
}

func classifyImpact(title string) string {
	title = strings.ToLower(title)
	highKeywords := []string{"nfp", "fomc", "interest rate", "cpi", "gdp", "employment", "payroll"}
	for _, kw := range highKeywords {
		if strings.Contains(title, kw) {
			return "high"
		}
	}
	medKeywords := []string{"pmi", "retail", "trade balance", "jobless"}
	for _, kw := range medKeywords {
		if strings.Contains(title, kw) {
			return "medium"
		}
	}
	return "low"
}

func extractCurrency(title string) string {
	currencies := []string{"USD", "EUR", "GBP", "JPY", "AUD", "NZD", "CAD", "CHF"}
	for _, c := range currencies {
		if strings.Contains(strings.ToUpper(title), c) {
			return c
		}
	}
	return "UNKNOWN"
}

func parseNewsCache(cached string) ([]NewsItem, error) {
	var items []NewsItem
	if err := json.Unmarshal([]byte(cached), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func normalizeFailPolicy(v string) FailPolicy {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(FailPolicyClosed):
		return FailPolicyClosed
	default:
		return FailPolicyOpen
	}
}
