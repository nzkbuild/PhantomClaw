package memory

import (
	"fmt"
	"strings"
)

// EchoRecall searches past lessons by keyword/tag for context injection (PRD §13).
// Returns lessons matching any of the keywords, ordered by weight.
type EchoRecall struct {
	db *DB
}

// NewEchoRecall creates an echo memory recall engine.
func NewEchoRecall(db *DB) *EchoRecall {
	return &EchoRecall{db: db}
}

// Search finds lessons matching the given keywords for a symbol.
// Returns top-K results by weight × recency.
func (er *EchoRecall) Search(symbol string, keywords []string, limit int) ([]Lesson, error) {
	if len(keywords) == 0 {
		return er.db.GetLessonsBySymbol(symbol, limit)
	}

	// Build LIKE conditions for keyword matching
	var conditions []string
	var args []any
	args = append(args, symbol)

	for _, kw := range keywords {
		conditions = append(conditions, "(lesson LIKE ? OR tags LIKE ?)")
		pattern := "%" + kw + "%"
		args = append(args, pattern, pattern)
	}

	query := fmt.Sprintf(`
		SELECT id, trade_id, symbol, session, lesson, tags, weight, created_at
		FROM lessons
		WHERE symbol = ? AND (%s)
		ORDER BY weight DESC, created_at DESC
		LIMIT ?`,
		strings.Join(conditions, " OR "),
	)
	args = append(args, limit)

	rows, err := er.db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("echo: search error: %w", err)
	}
	defer rows.Close()

	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.TradeID, &l.Symbol, &l.Session, &l.Lesson, &l.Tags, &l.Weight, &l.CreatedAt); err != nil {
			return nil, err
		}
		lessons = append(lessons, l)
	}
	return lessons, rows.Err()
}

// SearchAll searches all symbols for lessons matching keywords.
func (er *EchoRecall) SearchAll(keywords []string, limit int) ([]Lesson, error) {
	if len(keywords) == 0 {
		return nil, nil
	}

	var conditions []string
	var args []any
	for _, kw := range keywords {
		conditions = append(conditions, "(lesson LIKE ? OR tags LIKE ?)")
		pattern := "%" + kw + "%"
		args = append(args, pattern, pattern)
	}

	query := fmt.Sprintf(`
		SELECT id, trade_id, symbol, session, lesson, tags, weight, created_at
		FROM lessons
		WHERE %s
		ORDER BY weight DESC, created_at DESC
		LIMIT ?`,
		strings.Join(conditions, " OR "),
	)
	args = append(args, limit)

	rows, err := er.db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("echo: search all error: %w", err)
	}
	defer rows.Close()

	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.TradeID, &l.Symbol, &l.Session, &l.Lesson, &l.Tags, &l.Weight, &l.CreatedAt); err != nil {
			return nil, err
		}
		lessons = append(lessons, l)
	}
	return lessons, rows.Err()
}

// AdjustWeight modifies a lesson's weight (called during nightly review).
func (er *EchoRecall) AdjustWeight(lessonID int64, delta float64) error {
	_, err := er.db.conn.Exec(
		"UPDATE lessons SET weight = MAX(0, MIN(10, weight + ?)) WHERE id = ?",
		delta, lessonID,
	)
	return err
}
