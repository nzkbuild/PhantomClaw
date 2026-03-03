package market

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/health"
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
	limiter    *health.RateLimiter
	recovery   *health.Recovery
}

// FailPolicy controls how fetch/parsing failures are handled in safety checks.
type FailPolicy string

const (
	FailPolicyOpen   FailPolicy = "fail_open"
	FailPolicyClosed FailPolicy = "fail_closed"
)

// NewNewsFetcher creates a news fetcher with cache support.
func NewNewsFetcher(db *memory.DB, failPolicy string, limiter *health.RateLimiter, recovery *health.Recovery) *NewsFetcher {
	return &NewsFetcher{
		db:         db,
		client:     &http.Client{Timeout: 10 * time.Second},
		failPolicy: normalizeFailPolicy(failPolicy),
		limiter:    limiter,
		recovery:   recovery,
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

type forexFactoryWeeklyXML struct {
	XMLName xml.Name `xml:"weeklyevents"`
	Events  []struct {
		Title    string `xml:"title"`
		Country  string `xml:"country"`
		Currency string `xml:"currency"`
		Date     string `xml:"date"`
		Time     string `xml:"time"`
		Impact   string `xml:"impact"`
	} `xml:"event"`
}

type newsEndpoint struct {
	name   string
	url    string
	format string // rss | xml | json
}

var newsEndpoints = []newsEndpoint{
	{
		name:   "forexfactory_weekly_json",
		url:    "https://nfs.faireconomy.media/ff_calendar_thisweek.json",
		format: "json",
	},
	{
		name:   "forexfactory_weekly_xml",
		url:    "https://nfs.faireconomy.media/ff_calendar_thisweek.xml",
		format: "xml",
	},
	{
		name:   "forexfactory_rss",
		url:    "https://www.forexfactory.com/rss",
		format: "rss",
	},
}

// FetchNews retrieves today's economic events and caches them.
func (nf *NewsFetcher) FetchNews() ([]NewsItem, error) {
	// Check cache first (15-min TTL)
	cached, found, err := nf.db.GetCache("news_today")
	if err == nil && found {
		items, parseErr := parseNewsCache(cached)
		if parseErr == nil {
			if nf.recovery != nil {
				nf.recovery.RecordSuccess("forexfactory")
			}
			return items, nil
		}
	}

	if nf.limiter != nil && !nf.limiter.Allow("forexfactory") {
		wait := nf.limiter.WaitTime("forexfactory")
		err := fmt.Errorf("news: rate limited, retry in %s", wait)
		if nf.recovery != nil {
			nf.recovery.RecordError("forexfactory", err)
		}
		return nil, err
	}

	var lastErr error
	for _, endpoint := range newsEndpoints {
		items, fetchErr := nf.fetchFromEndpoint(endpoint)
		if fetchErr != nil {
			lastErr = fetchErr
			continue
		}
		if len(items) == 0 {
			lastErr = fmt.Errorf("news: %s returned no events", endpoint.name)
			continue
		}

		// Cache for 15 minutes
		if payload, marshalErr := json.Marshal(items); marshalErr == nil {
			_ = nf.db.SetCache("news_today", string(payload), endpoint.name, time.Now().Add(15*time.Minute))
		}
		if nf.recovery != nil {
			nf.recovery.RecordSuccess("forexfactory")
		}
		return items, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("news: no endpoint available")
	}
	if nf.recovery != nil {
		nf.recovery.RecordError("forexfactory", lastErr)
	}
	return nil, lastErr
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

func (nf *NewsFetcher) fetchFromEndpoint(endpoint newsEndpoint) ([]NewsItem, error) {
	resp, err := nf.client.Get(endpoint.url)
	if err != nil {
		return nil, fmt.Errorf("news: %s fetch error: %w", endpoint.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("news: %s unexpected status %d", endpoint.name, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("news: %s read error: %w", endpoint.name, err)
	}

	switch endpoint.format {
	case "rss":
		return parseForexFactoryRSS(body, endpoint.name)
	case "xml":
		return parseForexFactoryWeeklyXML(body, endpoint.name)
	case "json":
		return parseForexFactoryWeeklyJSON(body, endpoint.name)
	default:
		return nil, fmt.Errorf("news: unsupported endpoint format %q", endpoint.format)
	}
}

func parseForexFactoryRSS(body []byte, source string) ([]NewsItem, error) {
	var rss forexFactoryRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("news: parse rss: %w", err)
	}

	items := make([]NewsItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
		items = append(items, NewsItem{
			Title:    strings.TrimSpace(item.Title),
			Impact:   classifyImpact(item.Title),
			Currency: extractCurrency(item.Title),
			Time:     pubDate,
			Source:   source,
		})
	}
	return items, nil
}

func parseForexFactoryWeeklyXML(body []byte, source string) ([]NewsItem, error) {
	var weekly forexFactoryWeeklyXML
	if err := xml.Unmarshal(body, &weekly); err != nil {
		return nil, fmt.Errorf("news: parse weekly xml: %w", err)
	}

	items := make([]NewsItem, 0, len(weekly.Events))
	for _, event := range weekly.Events {
		title := strings.TrimSpace(event.Title)
		if title == "" {
			continue
		}
		currency := strings.TrimSpace(event.Currency)
		if currency == "" {
			currency = strings.TrimSpace(event.Country)
		}

		impact := normalizeImpact(strings.TrimSpace(event.Impact))
		if impact == "low" {
			impact = classifyImpact(title)
		}

		items = append(items, NewsItem{
			Title:    title,
			Impact:   impact,
			Currency: strings.ToUpper(currency),
			Time:     parseEventTimestamp(event.Date, event.Time),
			Source:   source,
		})
	}
	return items, nil
}

func parseForexFactoryWeeklyJSON(body []byte, source string) ([]NewsItem, error) {
	var records []map[string]any
	if err := json.Unmarshal(body, &records); err != nil {
		// Some feeds wrap entries inside an object.
		var wrapped map[string]any
		if unwrapErr := json.Unmarshal(body, &wrapped); unwrapErr != nil {
			return nil, fmt.Errorf("news: parse weekly json: %w", err)
		}
		for _, key := range []string{"events", "data", "calendar"} {
			raw, ok := wrapped[key]
			if !ok {
				continue
			}
			list, ok := raw.([]any)
			if !ok {
				continue
			}
			records = recordsFromAny(list)
			break
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("news: weekly json feed has no events")
	}

	items := make([]NewsItem, 0, len(records))
	for _, rec := range records {
		title := firstNonEmpty(mapString(rec, "title"), mapString(rec, "event"), mapString(rec, "name"))
		if title == "" {
			continue
		}

		currency := firstNonEmpty(mapString(rec, "currency"), mapString(rec, "country"), mapString(rec, "ccy"))
		impact := normalizeImpact(firstNonEmpty(mapString(rec, "impact"), mapString(rec, "impact_level")))
		if impact == "low" {
			impact = classifyImpact(title)
		}

		datePart := firstNonEmpty(mapString(rec, "date"), mapString(rec, "day"))
		timePart := mapString(rec, "time")
		if timePart == "" {
			timePart = mapString(rec, "datetime")
		}
		items = append(items, NewsItem{
			Title:    title,
			Impact:   impact,
			Currency: strings.ToUpper(strings.TrimSpace(currency)),
			Time:     parseEventTimestamp(datePart, timePart),
			Source:   source,
		})
	}
	return items, nil
}

func recordsFromAny(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if rec, ok := item.(map[string]any); ok {
			out = append(out, rec)
		}
	}
	return out
}

func mapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseEventTimestamp(datePart, timePart string) time.Time {
	datePart = strings.TrimSpace(datePart)
	timePart = strings.TrimSpace(timePart)

	layouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-01-2006",
		"2006/01/02",
		"02 Jan 2006 3:04pm",
		"Jan 2 2006 3:04pm",
		"Mon Jan 2 15:04:05 2006",
	}

	candidates := []string{}
	if datePart != "" && timePart != "" {
		candidates = append(candidates, datePart+" "+timePart)
	}
	if datePart != "" {
		candidates = append(candidates, datePart)
	}
	if timePart != "" {
		candidates = append(candidates, timePart)
	}

	for _, candidate := range candidates {
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, candidate); err == nil {
				return ts
			}
		}
	}
	return time.Now().UTC()
}

func normalizeImpact(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "high", "red", "3", "high impact expected":
		return "high"
	case "medium", "med", "orange", "2", "moderate":
		return "medium"
	default:
		return "low"
	}
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
