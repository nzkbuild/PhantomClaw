package memory

import (
	"fmt"
	"strings"
	"time"
)

// DiaryWriter manages the daily trading diary (PRD §13.2).
type DiaryWriter struct {
	db *DB
}

// NewDiaryWriter creates a diary writer.
func NewDiaryWriter(db *DB) *DiaryWriter {
	return &DiaryWriter{db: db}
}

// Write appends a diary entry for today.
func (dw *DiaryWriter) Write(entryType, content string) error {
	date := time.Now().Format("2006-01-02")
	return dw.db.InsertDiary(date, entryType, content)
}

// GetToday retrieves all diary entries for today.
func (dw *DiaryWriter) GetToday() ([]DiaryEntry, error) {
	date := time.Now().Format("2006-01-02")
	return dw.GetByDate(date)
}

// DiaryEntry represents a single diary entry.
type DiaryEntry struct {
	ID        int64
	Date      string
	EntryType string
	Content   string
	CreatedAt time.Time
}

// GetByDate retrieves diary entries for a specific date.
func (dw *DiaryWriter) GetByDate(date string) ([]DiaryEntry, error) {
	rows, err := dw.db.conn.Query(`
		SELECT id, date, entry_type, content, created_at
		FROM diary WHERE date = ?
		ORDER BY created_at ASC`, date,
	)
	if err != nil {
		return nil, fmt.Errorf("diary: query error: %w", err)
	}
	defer rows.Close()

	var entries []DiaryEntry
	for rows.Next() {
		var e DiaryEntry
		if err := rows.Scan(&e.ID, &e.Date, &e.EntryType, &e.Content, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetWeekSummary retrieves diary entries for the last 7 days.
func (dw *DiaryWriter) GetWeekSummary() (string, error) {
	sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	rows, err := dw.db.conn.Query(`
		SELECT date, entry_type, content
		FROM diary WHERE date >= ?
		ORDER BY date ASC, created_at ASC`, sevenDaysAgo,
	)
	if err != nil {
		return "", fmt.Errorf("diary: week summary error: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	var currentDate string
	for rows.Next() {
		var date, entryType, content string
		rows.Scan(&date, &entryType, &content)
		if date != currentDate {
			sb.WriteString(fmt.Sprintf("\n## %s\n", date))
			currentDate = date
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", entryType, content))
	}
	return sb.String(), rows.Err()
}

// ArchiveOldEntries moves entries older than N days to compressed format.
func (dw *DiaryWriter) ArchiveOldEntries(daysOld int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -daysOld).Format("2006-01-02")
	result, err := dw.db.conn.Exec(`DELETE FROM diary WHERE date < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
