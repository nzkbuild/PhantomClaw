package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Turn represents a single conversation turn (signal or decision) within a trading day.
type Turn struct {
	Timestamp time.Time `json:"ts"`
	Pair      string    `json:"pair"`
	Role      string    `json:"role"`     // "user" (EA signal) | "assistant" (agent decision)
	Content   string    `json:"content"`  // Summary of signal or decision reasoning
	Decision  string    `json:"decision"` // HOLD | PLACE_PENDING (for assistant turns only)
}

// SessionStore persists conversation turns as JSONL files, one per trading day.
// This gives the agent memory across signals within the same day.
type SessionStore struct {
	dir string
	mu  sync.Mutex
}

// NewSessionStore creates a session store that writes to the given directory.
func NewSessionStore(dir string) (*SessionStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("session: create dir: %w", err)
	}
	return &SessionStore{dir: dir}, nil
}

// Append writes a single turn to today's JSONL file.
func (s *SessionStore) Append(turn Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now()
	}

	path := s.todayPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("session: open file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(turn)
	if err != nil {
		return fmt.Errorf("session: marshal turn: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("session: write turn: %w", err)
	}
	return nil
}

// LoadToday reads the last maxTurns from today's session file.
func (s *SessionStore) LoadToday(maxTurns int) ([]Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readFile(s.todayPath(), maxTurns, "")
}

// LoadForPair reads the last maxTurns for a specific pair from today's session.
func (s *SessionStore) LoadForPair(pair string, maxTurns int) ([]Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readFile(s.todayPath(), maxTurns, pair)
}

// TodayTurnCount returns how many turns have been recorded today.
func (s *SessionStore) TodayTurnCount() int {
	turns, _ := s.LoadToday(9999)
	return len(turns)
}

// --- Internal ---

func (s *SessionStore) todayPath() string {
	date := time.Now().Format("2006-01-02")
	return filepath.Join(s.dir, date+".jsonl")
}

func (s *SessionStore) readFile(path string, maxTurns int, pairFilter string) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no turns yet today
		}
		return nil, fmt.Errorf("session: read file: %w", err)
	}
	defer f.Close()

	var allTurns []Turn
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var t Turn
		if err := json.Unmarshal(scanner.Bytes(), &t); err != nil {
			continue // skip malformed lines
		}
		if pairFilter != "" && t.Pair != pairFilter {
			continue
		}
		allTurns = append(allTurns, t)
	}

	// Return only the last maxTurns
	if len(allTurns) > maxTurns {
		allTurns = allTurns[len(allTurns)-maxTurns:]
	}
	return allTurns, nil
}
