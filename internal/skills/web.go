package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// webSearchSkill searches the web using DuckDuckGo HTML (no API key needed).
func webSearchSkill() *Skill {
	return &Skill{
		Name:        "web_search",
		Description: "Search the web for market news, economic events, or trading analysis. Returns top 5 results with title, snippet, and URL.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query, e.g. 'FOMC meeting results today', 'gold price forecast', 'ECB rate decision'",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("web_search: invalid args: %w", err)
			}
			if p.Query == "" {
				return `{"error":"query is required"}`, nil
			}

			// Use DuckDuckGo HTML lite
			searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(p.Query)
			req, _ := http.NewRequest("GET", searchURL, nil)
			req.Header.Set("User-Agent", "PhantomClaw/2.0 (trading bot)")

			resp, err := httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("web_search: HTTP error: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			results := parseDDGResults(string(body), 5)

			data, _ := json.Marshal(map[string]any{
				"query":   p.Query,
				"results": results,
			})
			return string(data), nil
		},
	}
}

// webFetchSkill fetches a URL and extracts text content (strips HTML).
func webFetchSkill() *Skill {
	return &Skill{
		Name:        "web_fetch",
		Description: "Fetch a web page and extract its text content. Use for reading news articles, economic calendars, or analysis pages. Returns max 2000 characters.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to fetch, e.g. 'https://www.forexfactory.com/calendar'",
				},
			},
			"required": []string{"url"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("web_fetch: invalid args: %w", err)
			}
			if p.URL == "" {
				return `{"error":"url is required"}`, nil
			}

			req, _ := http.NewRequest("GET", p.URL, nil)
			req.Header.Set("User-Agent", "PhantomClaw/2.0 (trading bot)")

			resp, err := httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("web_fetch: HTTP error: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			text := stripHTML(string(body))

			// Limit to 2000 chars to avoid context overflow
			if len(text) > 2000 {
				text = text[:2000] + "..."
			}

			data, _ := json.Marshal(map[string]any{
				"url":     p.URL,
				"content": text,
				"length":  len(text),
			})
			return string(data), nil
		},
	}
}

// --- HTML parsing helpers ---

var (
	reTitle   = regexp.MustCompile(`<a[^>]*class="result__a"[^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)
	reHref    = regexp.MustCompile(`href="([^"]*)"`)
	reTags    = regexp.MustCompile(`<[^>]*>`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

type searchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

func parseDDGResults(html string, max int) []searchResult {
	var results []searchResult

	titles := reTitle.FindAllStringSubmatch(html, max*2)
	snippets := reSnippet.FindAllStringSubmatch(html, max*2)

	for i := 0; i < len(titles) && i < max; i++ {
		title := stripTags(titles[i][1])
		snippet := ""
		if i < len(snippets) {
			snippet = stripTags(snippets[i][1])
		}

		// Extract URL from the title link
		linkURL := ""
		if match := reHref.FindStringSubmatch(titles[i][0]); len(match) > 1 {
			linkURL = match[1]
			// DDG wraps URLs — extract actual URL if present
			if idx := strings.Index(linkURL, "uddg="); idx >= 0 {
				decoded, err := url.QueryUnescape(linkURL[idx+5:])
				if err == nil {
					linkURL = decoded
				}
			}
		}

		results = append(results, searchResult{
			Title:   title,
			Snippet: snippet,
			URL:     linkURL,
		})
	}
	return results
}

func stripHTML(html string) string {
	text := reTags.ReplaceAllString(html, " ")
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func stripTags(s string) string {
	return strings.TrimSpace(reTags.ReplaceAllString(s, ""))
}
