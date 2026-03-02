package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite connection for PhantomClaw memory.
type DB struct {
	conn *sql.DB
}

// NewDB opens (or creates) the SQLite database at the given path,
// runs the schema migration, and returns a ready DB handle.
func NewDB(dbPath string) (*DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("memory: create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}

	// Enable WAL mode for concurrent read/write
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("memory: set WAL mode: %w", err)
	}

	// Run schema migration
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("memory: run schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// QueryRow exposes a single-row SQL query for flexible lookups.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// QueryRows exposes a multi-row SQL query for flexible lookups.
func (db *DB) QueryRows(query string, args ...any) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// --- Trade Operations ---

// Trade represents a single trade record.
type Trade struct {
	ID         int64
	Symbol     string
	Direction  string
	Entry      float64
	Exit       float64
	Lot        float64
	SL         float64
	TP         float64
	PnL        float64
	Session    string
	Timeframe  string
	LLMReason  string
	Confidence int
	OpenedAt   time.Time
	ClosedAt   *time.Time
}

// InsertTrade inserts a new trade (at open time) and returns the trade ID.
func (db *DB) InsertTrade(t *Trade) (int64, error) {
	res, err := db.conn.Exec(`
		INSERT INTO trades (symbol, direction, entry, lot, sl, tp, session, timeframe, llm_reason, confidence, opened_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Symbol, t.Direction, t.Entry, t.Lot, t.SL, t.TP,
		t.Session, t.Timeframe, t.LLMReason, t.Confidence, t.OpenedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: insert trade: %w", err)
	}
	return res.LastInsertId()
}

// CloseTrade updates a trade with exit price, PnL, and closed timestamp.
func (db *DB) CloseTrade(id int64, exit, pnl float64, closedAt time.Time) error {
	_, err := db.conn.Exec(`
		UPDATE trades SET exit = ?, pnl = ?, closed_at = ? WHERE id = ?`,
		exit, pnl, closedAt, id,
	)
	return err
}

// --- Lesson Operations ---

// Lesson represents a post-trade lesson.
type Lesson struct {
	ID        int64
	TradeID   int64
	Symbol    string
	Session   string
	Lesson    string
	Tags      string // JSON array
	Weight    float64
	CreatedAt time.Time
}

// InsertLesson stores a new lesson linked to a trade.
func (db *DB) InsertLesson(l *Lesson) (int64, error) {
	res, err := db.conn.Exec(`
		INSERT INTO lessons (trade_id, symbol, session, lesson, tags, weight)
		VALUES (?, ?, ?, ?, ?, ?)`,
		l.TradeID, l.Symbol, l.Session, l.Lesson, l.Tags, l.Weight,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: insert lesson: %w", err)
	}
	return res.LastInsertId()
}

// GetLessonsBySymbol retrieves the top-K lessons for a symbol, ordered by weight and recency.
func (db *DB) GetLessonsBySymbol(symbol string, limit int) ([]Lesson, error) {
	rows, err := db.conn.Query(`
		SELECT id, trade_id, symbol, session, lesson, tags, weight, created_at
		FROM lessons WHERE symbol = ?
		ORDER BY weight DESC, created_at DESC
		LIMIT ?`, symbol, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get lessons: %w", err)
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

// --- Pair State Operations ---

// UpsertPairState inserts or updates the adaptive state for a trading pair.
func (db *DB) UpsertPairState(symbol, bias, preferredTF string, winRate, avgPnL float64, sessionScores string) error {
	_, err := db.conn.Exec(`
		INSERT INTO pair_state (symbol, bias, preferred_tf, win_rate_7d, avg_pnl_7d, session_scores, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(symbol) DO UPDATE SET
			bias = excluded.bias,
			preferred_tf = excluded.preferred_tf,
			win_rate_7d = excluded.win_rate_7d,
			avg_pnl_7d = excluded.avg_pnl_7d,
			session_scores = excluded.session_scores,
			updated_at = datetime('now')`,
		symbol, bias, preferredTF, winRate, avgPnL, sessionScores,
	)
	return err
}

// --- Session RAM Operations ---

// SetSessionRAM sets a key-value pair in session RAM with expiry.
func (db *DB) SetSessionRAM(key, value string, expiresAt time.Time) error {
	_, err := db.conn.Exec(`
		INSERT INTO session_ram (key, value, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, value, expiresAt,
	)
	return err
}

// GetSessionRAM retrieves a value from session RAM (returns empty if expired or missing).
func (db *DB) GetSessionRAM(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(`
		SELECT value FROM session_ram WHERE key = ? AND expires_at > datetime('now')`,
		key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ClearSessionRAM deletes all session RAM entries (called at 00:00 MYT).
func (db *DB) ClearSessionRAM() error {
	_, err := db.conn.Exec("DELETE FROM session_ram")
	return err
}

// --- Diary Operations ---

// InsertDiary appends a diary entry.
func (db *DB) InsertDiary(date, entryType, content string) error {
	_, err := db.conn.Exec(`
		INSERT INTO diary (date, entry_type, content) VALUES (?, ?, ?)`,
		date, entryType, content,
	)
	return err
}

// --- Cache Operations ---

// SetCache stores a cached value with TTL.
func (db *DB) SetCache(key, value, source string, expiresAt time.Time) error {
	_, err := db.conn.Exec(`
		INSERT INTO market_cache (key, value, source, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, source = excluded.source, expires_at = excluded.expires_at`,
		key, value, source, expiresAt,
	)
	return err
}

// GetCache retrieves a cached value if not expired.
func (db *DB) GetCache(key string) (string, bool, error) {
	var value string
	err := db.conn.QueryRow(`
		SELECT value FROM market_cache WHERE key = ? AND expires_at > datetime('now')`,
		key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// PruneExpiredCache removes all expired cache entries.
func (db *DB) PruneExpiredCache() error {
	_, err := db.conn.Exec("DELETE FROM market_cache WHERE expires_at <= datetime('now')")
	return err
}

// --- Pending Decision Operations ---

// UpsertPendingDecision stores or updates a pending bridge decision.
func (db *DB) UpsertPendingDecision(requestID, symbol, decisionJSON string, expiresAt time.Time) error {
	_, err := db.conn.Exec(`
		INSERT INTO pending_decisions (request_id, symbol, decision_json, status, expires_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, datetime('now'))
		ON CONFLICT(request_id) DO UPDATE SET
			symbol = excluded.symbol,
			decision_json = excluded.decision_json,
			status = 'pending',
			expires_at = excluded.expires_at,
			updated_at = datetime('now')`,
		requestID, symbol, decisionJSON, expiresAt,
	)
	return err
}

// GetPendingDecisionByRequestID retrieves an unconsumed, unexpired decision payload by request_id.
func (db *DB) GetPendingDecisionByRequestID(requestID string) (string, bool, error) {
	var decisionJSON string
	err := db.conn.QueryRow(`
		SELECT decision_json
		FROM pending_decisions
		WHERE request_id = ?
		  AND status = 'pending'
		  AND expires_at > datetime('now')`,
		requestID,
	).Scan(&decisionJSON)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return decisionJSON, true, nil
}

// GetPendingDecisionBySymbol retrieves the latest unconsumed, unexpired decision payload for a symbol.
func (db *DB) GetPendingDecisionBySymbol(symbol string) (string, string, bool, error) {
	var requestID string
	var decisionJSON string
	err := db.conn.QueryRow(`
		SELECT request_id, decision_json
		FROM pending_decisions
		WHERE symbol = ?
		  AND status = 'pending'
		  AND expires_at > datetime('now')
		ORDER BY updated_at DESC
		LIMIT 1`,
		symbol,
	).Scan(&requestID, &decisionJSON)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return requestID, decisionJSON, true, nil
}

// ConsumePendingDecision marks a pending decision as consumed.
func (db *DB) ConsumePendingDecision(requestID string) error {
	_, err := db.conn.Exec(`
		UPDATE pending_decisions
		SET status = 'consumed', updated_at = datetime('now')
		WHERE request_id = ? AND status = 'pending'`,
		requestID,
	)
	return err
}

// ExpirePendingDecisions marks all expired pending decisions as expired.
func (db *DB) ExpirePendingDecisions(now time.Time) error {
	_, err := db.conn.Exec(`
		UPDATE pending_decisions
		SET status = 'expired', updated_at = datetime('now')
		WHERE status = 'pending' AND expires_at <= ?`,
		now,
	)
	return err
}
