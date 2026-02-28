package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

// SentimentResult holds the sentiment analysis for a symbol.
type SentimentResult struct {
	Symbol    string    `json:"symbol"`
	Bullish   int       `json:"bullish"`
	Bearish   int       `json:"bearish"`
	Neutral   int       `json:"neutral"`
	Score     float64   `json:"score"` // -1.0 (very bearish) to +1.0 (very bullish)
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SentimentFetcher gathers sentiment from Reddit.
type SentimentFetcher struct {
	db     *memory.DB
	client *http.Client
}

// NewSentimentFetcher creates a sentiment fetcher.
func NewSentimentFetcher(db *memory.DB) *SentimentFetcher {
	return &SentimentFetcher{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchSentiment gets sentiment for a symbol from Reddit RSS.
func (sf *SentimentFetcher) FetchSentiment(symbol string) (*SentimentResult, error) {
	// Check cache (30-min TTL)
	cacheKey := "sentiment_" + symbol
	cached, found, err := sf.db.GetCache(cacheKey)
	if err == nil && found {
		var result SentimentResult
		if json.Unmarshal([]byte(cached), &result) == nil {
			return &result, nil
		}
	}

	// Fetch from Reddit RSS (r/Forex)
	query := strings.ToLower(symbol)
	url := fmt.Sprintf("https://www.reddit.com/r/Forex/search.json?q=%s&sort=new&limit=25&restrict_sr=1", query)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "PhantomClaw/1.0 (trading bot)")

	resp, err := sf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sentiment: fetch error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var redditResp struct {
		Data struct {
			Children []struct {
				Data struct {
					Title string `json:"title"`
					Score int    `json:"score"`
					Ups   int    `json:"ups"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &redditResp); err != nil {
		return nil, fmt.Errorf("sentiment: parse error: %w", err)
	}

	// Simple keyword sentiment analysis
	var bullish, bearish, neutral int
	for _, child := range redditResp.Data.Children {
		title := strings.ToLower(child.Data.Title)
		switch {
		case containsAny(title, "buy", "long", "bullish", "support", "breakout", "moon"):
			bullish++
		case containsAny(title, "sell", "short", "bearish", "resistance", "breakdown", "crash"):
			bearish++
		default:
			neutral++
		}
	}

	total := bullish + bearish + neutral
	score := 0.0
	if total > 0 {
		score = float64(bullish-bearish) / float64(total)
	}

	result := &SentimentResult{
		Symbol:    symbol,
		Bullish:   bullish,
		Bearish:   bearish,
		Neutral:   neutral,
		Score:     score,
		Source:    "reddit",
		UpdatedAt: time.Now(),
	}

	// Cache for 30 minutes
	resultJSON, _ := json.Marshal(result)
	sf.db.SetCache(cacheKey, string(resultJSON), "reddit", time.Now().Add(30*time.Minute))

	return result, nil
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
