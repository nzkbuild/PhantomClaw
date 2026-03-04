package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// ChatTurn represents a single message in a chat conversation.
type ChatTurn struct {
	Timestamp time.Time `json:"ts"`
	Role      string    `json:"role"`    // "user" | "assistant"
	Content   string    `json:"content"` // The message text
}

// ChatHistory persists chat conversation turns in SQLite.
// Survives restarts and auto-prunes to stay within a configurable limit.
type ChatHistory struct {
	db       *sql.DB
	maxTurns int
}

// NewChatHistory creates a persistent chat history backed by the conversations table.
// maxTurns controls how many turns are kept (oldest auto-pruned).
func NewChatHistory(maxTurns int) *ChatHistory {
	if maxTurns <= 0 {
		maxTurns = 40
	}
	return &ChatHistory{maxTurns: maxTurns}
}

// Bind connects the chat history to a database connection.
// Must be called before Append/Recent. Safe to call with nil (becomes no-op).
func (ch *ChatHistory) Bind(db *sql.DB) {
	ch.db = db
}

// Append stores a turn in the conversations table and prunes old entries.
func (ch *ChatHistory) Append(role, content string) {
	if ch.db == nil {
		return
	}

	_, err := ch.db.Exec(
		`INSERT INTO conversations (role, content) VALUES (?, ?)`,
		role, content,
	)
	if err != nil {
		return // silent fail — chat memory is non-critical
	}

	// Prune old turns beyond maxTurns
	ch.db.Exec(`
		DELETE FROM conversations WHERE id NOT IN (
			SELECT id FROM conversations ORDER BY id DESC LIMIT ?
		)`, ch.maxTurns)
}

// Recent returns the last N chat turns from the database.
func (ch *ChatHistory) Recent(n int) []ChatTurn {
	if ch.db == nil {
		return nil
	}
	if n <= 0 {
		n = ch.maxTurns
	}

	rows, err := ch.db.Query(`
		SELECT role, content, created_at FROM conversations
		ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var turns []ChatTurn
	for rows.Next() {
		var t ChatTurn
		if err := rows.Scan(&t.Role, &t.Content, &t.Timestamp); err != nil {
			continue
		}
		turns = append(turns, t)
	}

	// Reverse so oldest is first (SQL returns newest first)
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns
}

// Clear removes all chat history.
func (ch *ChatHistory) Clear() {
	if ch.db == nil {
		return
	}
	ch.db.Exec("DELETE FROM conversations")
}

// Len returns the current number of stored turns.
func (ch *ChatHistory) Len() int {
	if ch.db == nil {
		return 0
	}
	var count int
	err := ch.db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// PruneOlderThanDays removes chat turns older than N days.
func (ch *ChatHistory) PruneOlderThanDays(days int) (int, error) {
	if ch.db == nil || days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := ch.db.Exec(
		`DELETE FROM conversations WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("chat: prune: %w", err)
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}
