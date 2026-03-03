package logging

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Query defines filters for structured log retrieval.
type Query struct {
	Level     string
	Component string
	Contains  string
	Since     time.Time
	Limit     int
}

// QueryJSONLogFile reads zap JSON logs with simple filters.
func QueryJSONLogFile(path string, q Query) ([]map[string]any, error) {
	if q.Limit <= 0 {
		q.Limit = 200
	}
	if q.Limit > 5000 {
		q.Limit = 5000
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	defer file.Close()

	levelFilter := strings.ToLower(strings.TrimSpace(q.Level))
	componentFilter := strings.ToLower(strings.TrimSpace(q.Component))
	containsFilter := strings.ToLower(strings.TrimSpace(q.Contains))

	rows := make([]map[string]any, 0, q.Limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if !matchesFilters(entry, levelFilter, componentFilter, containsFilter, q.Since) {
			continue
		}

		rows = append(rows, entry)
		if len(rows) > q.Limit {
			rows = rows[len(rows)-q.Limit:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func matchesFilters(entry map[string]any, levelFilter, componentFilter, containsFilter string, since time.Time) bool {
	level := strings.ToLower(getString(entry["level"]))
	if levelFilter != "" && level != levelFilter {
		return false
	}

	component := strings.ToLower(getString(entry["caller"]))
	if componentFilter != "" && !strings.Contains(component, componentFilter) {
		return false
	}

	if containsFilter != "" {
		blob := strings.ToLower(getString(entry["msg"])) + " " + strings.ToLower(getString(entry["caller"]))
		if !strings.Contains(blob, containsFilter) {
			return false
		}
	}

	if !since.IsZero() {
		tsRaw := getString(entry["ts"])
		if tsRaw != "" {
			if parsed, err := time.Parse(time.RFC3339, tsRaw); err == nil && parsed.Before(since) {
				return false
			}
			if parsed, err := time.Parse(time.RFC3339Nano, tsRaw); err == nil && parsed.Before(since) {
				return false
			}
			if parsed, err := time.Parse("2006-01-02T15:04:05.000Z0700", tsRaw); err == nil && parsed.Before(since) {
				return false
			}
		}
	}

	return true
}

func getString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
