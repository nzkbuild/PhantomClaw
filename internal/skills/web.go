package skills

import (
	"encoding/json"
	"fmt"
	htmlstd "html"
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
	reAnchor = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	reHref   = regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	reTags   = regexp.MustCompile(`<[^>]*>`)
	reSpaces = regexp.MustCompile(`\s+`)
)

type searchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

func parseDDGResults(html string, max int) []searchResult {
	var results []searchResult
	seen := make(map[string]struct{})
	matches := reAnchor.FindAllStringSubmatchIndex(html, -1)

	for _, m := range matches {
		if len(results) >= max {
			break
		}
		// Match index layout:
		// 0,1 = full anchor; 2,3 = attrs; 4,5 = inner HTML
		if len(m) < 6 {
			continue
		}

		attrs := html[m[2]:m[3]]
		rawTitle := html[m[4]:m[5]]

		href := extractHref(attrs)
		linkURL := normalizeDDGURL(href)
		if !isLikelyResultURL(linkURL) {
			continue
		}

		title := stripTags(rawTitle)
		if title == "" || isLikelyNavTitle(title) {
			continue
		}

		if _, exists := seen[linkURL]; exists {
			continue
		}
		seen[linkURL] = struct{}{}

		snippet := extractSnippetAfterAnchor(html, m[1], title)
		results = append(results, searchResult{
			Title:   title,
			Snippet: snippet,
			URL:     linkURL,
		})
	}
	return results
}

func extractHref(attrs string) string {
	match := reHref.FindStringSubmatch(attrs)
	if len(match) < 2 {
		return ""
	}
	for i := 1; i < len(match); i++ {
		if match[i] != "" {
			return htmlstd.UnescapeString(strings.TrimSpace(match[i]))
		}
	}
	return ""
}

func normalizeDDGURL(href string) string {
	if href == "" {
		return ""
	}

	// DDG redirect style: /l/?kh=-1&uddg=<encoded-url>
	if strings.Contains(href, "uddg=") {
		u, err := url.Parse(href)
		if err == nil {
			uddg := u.Query().Get("uddg")
			if uddg != "" {
				decoded, decErr := url.QueryUnescape(uddg)
				if decErr == nil {
					href = decoded
				}
			}
		}
	}

	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	return href
}

func isLikelyResultURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(strings.ToLower(rawURL), "javascript:") {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, "duckduckgo.com") {
		return false
	}
	return true
}

func isLikelyNavTitle(title string) bool {
	lower := strings.ToLower(strings.TrimSpace(title))
	switch lower {
	case "more results", "next", "feedback", "settings", "help", "privacy":
		return true
	}
	return false
}

func extractSnippetAfterAnchor(html string, anchorEnd int, title string) string {
	if anchorEnd >= len(html) {
		return ""
	}
	end := anchorEnd + 500
	if end > len(html) {
		end = len(html)
	}
	text := stripHTML(html[anchorEnd:end])
	text = strings.TrimPrefix(text, title)
	text = strings.TrimSpace(text)
	if len(text) > 220 {
		text = text[:220]
	}
	return text
}

func stripHTML(html string) string {
	text := reTags.ReplaceAllString(html, " ")
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func stripTags(s string) string {
	return strings.TrimSpace(reTags.ReplaceAllString(s, ""))
}
