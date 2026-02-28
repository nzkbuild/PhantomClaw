package market

import (
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
	db     *memory.DB
	client *http.Client
}

// NewNewsFetcher creates a news fetcher with cache support.
func NewNewsFetcher(db *memory.DB) *NewsFetcher {
	return &NewsFetcher{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
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
		return parseNewsCache(cached), nil
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
		items = append(items, NewsItem{
			Title:    item.Title,
			Impact:   impact,
			Currency: currency,
			Source:   "forexfactory",
		})
	}

	// Cache for 15 minutes
	nf.db.SetCache("news_today", fmt.Sprintf("%v", items), "forexfactory",
		time.Now().Add(15*time.Minute))

	return items, nil
}

// HasHighImpactEvent checks if there's a high-impact event for the given currency.
func (nf *NewsFetcher) HasHighImpactEvent(currency string) bool {
	items, err := nf.FetchNews()
	if err != nil {
		return false // Fail open — don't block on news fetch errors
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

func parseNewsCache(cached string) []NewsItem {
	// Simplified — in production, cache as JSON
	return nil
}
