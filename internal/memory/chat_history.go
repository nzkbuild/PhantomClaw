package memory

import (
	"sync"
	"time"
)

// ChatTurn represents a single message in a chat conversation (not a trading signal).
type ChatTurn struct {
	Timestamp time.Time `json:"ts"`
	Role      string    `json:"role"`    // "user" | "assistant"
	Content   string    `json:"content"` // The message text
}

// ChatHistory maintains a bounded in-memory history of chat conversation turns.
// This gives the Telegram chat mode memory across messages within the same session.
type ChatHistory struct {
	mu       sync.Mutex
	turns    []ChatTurn
	maxTurns int
}

// NewChatHistory creates a chat history buffer with the given max turn limit.
func NewChatHistory(maxTurns int) *ChatHistory {
	if maxTurns <= 0 {
		maxTurns = 20
	}
	return &ChatHistory{
		turns:    make([]ChatTurn, 0, maxTurns),
		maxTurns: maxTurns,
	}
}

// Append adds a turn to the chat history, evicting the oldest if at capacity.
func (ch *ChatHistory) Append(role, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.turns = append(ch.turns, ChatTurn{
		Timestamp: time.Now(),
		Role:      role,
		Content:   content,
	})

	// Evict oldest if over capacity
	if len(ch.turns) > ch.maxTurns {
		ch.turns = ch.turns[len(ch.turns)-ch.maxTurns:]
	}
}

// Recent returns the last N chat turns (or all if fewer exist).
func (ch *ChatHistory) Recent(n int) []ChatTurn {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if n <= 0 || n > len(ch.turns) {
		n = len(ch.turns)
	}
	result := make([]ChatTurn, n)
	copy(result, ch.turns[len(ch.turns)-n:])
	return result
}

// Clear resets the chat history.
func (ch *ChatHistory) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.turns = ch.turns[:0]
}

// Len returns the current number of stored turns.
func (ch *ChatHistory) Len() int {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return len(ch.turns)
}
