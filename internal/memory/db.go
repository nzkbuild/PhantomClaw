package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite connection for PhantomClaw memory.
type DB struct {
	conn *sql.DB
}

const currentSchemaVersion = 1

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

	if err := ensureSchemaVersion(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("memory: schema version check failed: %w", err)
	}

	return &DB{conn: conn}, nil
}

// NewReadOnlyDB opens a read-only connection to an existing SQLite database.
// Use this for dashboard/API queries to avoid contention with the main write connection.
func NewReadOnlyDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("memory: open read-only db: %w", err)
	}
	conn.SetMaxOpenConns(2)
	conn.SetMaxIdleConns(1)
	// Verify connectivity
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("memory: read-only db ping failed: %w", err)
	}
	return &DB{conn: conn}, nil
}

func ensureSchemaVersion(conn *sql.DB) error {
	var value string
	err := conn.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&value)
	if err == sql.ErrNoRows {
		_, insErr := conn.Exec(`
			INSERT INTO metadata (key, value, updated_at)
			VALUES ('schema_version', ?, datetime('now'))`,
			strconv.Itoa(currentSchemaVersion),
		)
		return insErr
	}
	if err != nil {
		return err
	}

	existing, convErr := strconv.Atoi(value)
	if convErr != nil {
		return fmt.Errorf("invalid schema_version value: %q", value)
	}
	if existing != currentSchemaVersion {
		return fmt.Errorf("expected schema_version=%d but found=%d", currentSchemaVersion, existing)
	}
	return nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB for direct access (e.g., ChatHistory binding).
func (db *DB) Conn() *sql.DB {
	return db.conn
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

// GetPendingDecisionByRequestID retrieves an active (pending/delivered) unexpired decision by request_id.
func (db *DB) GetPendingDecisionByRequestID(requestID string) (string, string, bool, error) {
	var decisionJSON string
	var currentStatus string
	err := db.conn.QueryRow(`
		SELECT decision_json, status
		FROM pending_decisions
		WHERE request_id = ?
		  AND status IN ('pending', 'delivered')
		  AND expires_at > datetime('now')`,
		requestID,
	).Scan(&decisionJSON, &currentStatus)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return decisionJSON, currentStatus, true, nil
}

// GetPendingDecisionBySymbol retrieves the latest active (pending/delivered) unexpired decision payload for a symbol.
func (db *DB) GetPendingDecisionBySymbol(symbol string) (string, string, string, bool, error) {
	var requestID string
	var decisionJSON string
	var currentStatus string
	err := db.conn.QueryRow(`
		SELECT request_id, decision_json, status
		FROM pending_decisions
		WHERE symbol = ?
		  AND status IN ('pending', 'delivered')
		  AND expires_at > datetime('now')
		ORDER BY updated_at DESC
		LIMIT 1`,
		symbol,
	).Scan(&requestID, &decisionJSON, &currentStatus)
	if err == sql.ErrNoRows {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, err
	}
	return requestID, decisionJSON, currentStatus, true, nil
}

// MarkPendingDecisionDelivered marks a pending decision as delivered.
func (db *DB) MarkPendingDecisionDelivered(requestID string) error {
	_, err := db.conn.Exec(`
		UPDATE pending_decisions
		SET status = 'delivered', updated_at = datetime('now')
		WHERE request_id = ? AND status = 'pending'`,
		requestID,
	)
	return err
}

// ConsumePendingDecision marks a pending decision as consumed.
func (db *DB) ConsumePendingDecision(requestID string) error {
	_, err := db.conn.Exec(`
		UPDATE pending_decisions
		SET status = 'consumed', updated_at = datetime('now')
		WHERE request_id = ? AND status IN ('pending', 'delivered')`,
		requestID,
	)
	return err
}

// ExpirePendingDecisions marks all expired pending decisions as expired.
func (db *DB) ExpirePendingDecisions(now time.Time) error {
	_, err := db.conn.Exec(`
		UPDATE pending_decisions
		SET status = 'expired', updated_at = datetime('now')
		WHERE status IN ('pending', 'delivered') AND expires_at <= ?`,
		now,
	)
	return err
}

// --- Durable Cron Job Operations ---

// CronJob is a persisted one-shot recheck job scheduled by cron_add.
type CronJob struct {
	JobID     string
	Pair      string
	Reason    string
	WakeAt    time.Time
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UpsertCronJob inserts or updates a cron job and sets it to pending.
func (db *DB) UpsertCronJob(jobID, pair, reason string, wakeAt time.Time) error {
	_, err := db.conn.Exec(`
		INSERT INTO cron_jobs (job_id, pair, reason, wake_at, status, updated_at)
		VALUES (?, ?, ?, ?, 'pending', datetime('now'))
		ON CONFLICT(job_id) DO UPDATE SET
			pair = excluded.pair,
			reason = excluded.reason,
			wake_at = excluded.wake_at,
			status = 'pending',
			updated_at = datetime('now')`,
		jobID, pair, reason, wakeAt,
	)
	return err
}

// ListPendingCronJobs returns all pending cron jobs ordered by wake time.
func (db *DB) ListPendingCronJobs() ([]CronJob, error) {
	rows, err := db.conn.Query(`
		SELECT job_id, pair, reason, wake_at, status, created_at, updated_at
		FROM cron_jobs
		WHERE status = 'pending'
		ORDER BY wake_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []CronJob
	for rows.Next() {
		var job CronJob
		if err := rows.Scan(
			&job.JobID,
			&job.Pair,
			&job.Reason,
			&job.WakeAt,
			&job.Status,
			&job.CreatedAt,
			&job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// MarkCronJobFired marks a pending cron job as fired.
func (db *DB) MarkCronJobFired(jobID string) error {
	_, err := db.conn.Exec(`
		UPDATE cron_jobs
		SET status = 'fired', updated_at = datetime('now')
		WHERE job_id = ? AND status = 'pending'`,
		jobID,
	)
	return err
}

// PendingDecisionRecord is an inspectable queue entry for admin endpoints.
type PendingDecisionRecord struct {
	RequestID string    `json:"request_id"`
	Symbol    string    `json:"symbol"`
	Status    string    `json:"status"`
	Decision  string    `json:"decision,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListActivePendingDecisions lists non-consumed queue entries, newest first.
func (db *DB) ListActivePendingDecisions(limit int) ([]PendingDecisionRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(`
		SELECT request_id, symbol, status, updated_at, expires_at
		FROM pending_decisions
		WHERE status IN ('pending', 'delivered')
		ORDER BY updated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingDecisionRecord
	for rows.Next() {
		var row PendingDecisionRecord
		if err := rows.Scan(&row.RequestID, &row.Symbol, &row.Status, &row.UpdatedAt, &row.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// DecisionHistoryRecord is a dashboard-friendly decision history item.
type DecisionHistoryRecord struct {
	RequestID string    `json:"request_id"`
	Symbol    string    `json:"symbol"`
	Status    string    `json:"status"`
	Decision  string    `json:"decision,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListDecisionHistory returns latest decisions (including consumed/expired) for history browsing.
func (db *DB) ListDecisionHistory(limit int, symbol string) ([]DecisionHistoryRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	args := []any{}
	query := `
		SELECT request_id, symbol, status, decision_json, created_at, updated_at, expires_at
		FROM pending_decisions`
	if symbol != "" {
		query += " WHERE symbol = ?"
		args = append(args, symbol)
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DecisionHistoryRecord
	for rows.Next() {
		var (
			entry   DecisionHistoryRecord
			payload string
		)
		if err := rows.Scan(
			&entry.RequestID,
			&entry.Symbol,
			&entry.Status,
			&payload,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.ExpiresAt,
		); err != nil {
			return nil, err
		}

		var parsed struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(payload), &parsed); err == nil {
			entry.Decision = parsed.Action
			entry.Reason = parsed.Reason
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// TradeSummary is an aggregate metrics snapshot for dashboard reporting.
type TradeSummary struct {
	Days          int     `json:"days"`
	TotalTrades   int     `json:"total_trades"`
	Wins          int     `json:"wins"`
	Losses        int     `json:"losses"`
	WinRate       float64 `json:"win_rate"`
	TotalPnL      float64 `json:"total_pnl"`
	AveragePnL    float64 `json:"average_pnl"`
	MaxDrawdownPn float64 `json:"max_drawdown_pn"`
}

// EquityPoint is a single cumulative P&L data point for the equity curve.
type EquityPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// PairAnalytics holds per-symbol performance breakdown.
type PairAnalytics struct {
	Symbol     string  `json:"symbol"`
	Trades     int     `json:"trades"`
	Wins       int     `json:"wins"`
	WinRate    float64 `json:"win_rate"`
	TotalPnL   float64 `json:"total_pnl"`
	AveragePnL float64 `json:"average_pnl"`
}

// GetTradeSummary returns aggregate trade metrics over the last N days.
func (db *DB) GetTradeSummary(days int) (*TradeSummary, error) {
	if days <= 0 {
		days = 30
	}
	start := time.Now().AddDate(0, 0, -days)

	rows, err := db.conn.Query(`
		SELECT COALESCE(pnl, 0)
		FROM trades
		WHERE closed_at IS NOT NULL AND closed_at >= ?
		ORDER BY closed_at ASC`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &TradeSummary{Days: days}
	equityCurve := 0.0
	equityPeak := 0.0
	for rows.Next() {
		var pnl float64
		if err := rows.Scan(&pnl); err != nil {
			return nil, err
		}
		summary.TotalTrades++
		summary.TotalPnL += pnl
		if pnl > 0 {
			summary.Wins++
		} else if pnl < 0 {
			summary.Losses++
		}

		equityCurve += pnl
		if equityCurve > equityPeak {
			equityPeak = equityCurve
		}
		drawdown := equityPeak - equityCurve
		if drawdown > summary.MaxDrawdownPn {
			summary.MaxDrawdownPn = drawdown
		}
	}
	if summary.TotalTrades > 0 {
		summary.WinRate = float64(summary.Wins) / float64(summary.TotalTrades)
		summary.AveragePnL = summary.TotalPnL / float64(summary.TotalTrades)
	}
	return summary, rows.Err()
}

// GetEquityCurve returns a time-ordered series of cumulative P&L points
// for closed trades within the last N days. Used by the equity curve chart.
func (db *DB) GetEquityCurve(days int) ([]EquityPoint, error) {
	if days <= 0 {
		days = 30
	}
	start := time.Now().AddDate(0, 0, -days)

	rows, err := db.conn.Query(`
		SELECT closed_at, COALESCE(pnl, 0)
		FROM trades
		WHERE closed_at IS NOT NULL AND closed_at >= ?
		ORDER BY closed_at ASC`, start)
	if err != nil {
		return nil, fmt.Errorf("memory: equity curve query: %w", err)
	}
	defer rows.Close()

	var points []EquityPoint
	cumulative := 0.0
	for rows.Next() {
		var ts time.Time
		var pnl float64
		if err := rows.Scan(&ts, &pnl); err != nil {
			return nil, err
		}
		cumulative += pnl
		points = append(points, EquityPoint{
			Time:  ts.UTC().Format(time.RFC3339),
			Value: math.Round(cumulative*100) / 100,
		})
	}
	if points == nil {
		points = []EquityPoint{}
	}
	return points, rows.Err()
}

// GetPairAnalytics returns per-symbol performance breakdown for closed trades
// within the last N days, ordered by total P&L descending.
func (db *DB) GetPairAnalytics(days int) ([]PairAnalytics, error) {
	if days <= 0 {
		days = 30
	}
	start := time.Now().AddDate(0, 0, -days)

	rows, err := db.conn.Query(`
		SELECT
			symbol,
			COUNT(*) AS trades,
			SUM(CASE WHEN pnl > 0 THEN 1 ELSE 0 END) AS wins,
			SUM(COALESCE(pnl, 0)) AS total_pnl,
			AVG(COALESCE(pnl, 0)) AS avg_pnl
		FROM trades
		WHERE closed_at IS NOT NULL AND closed_at >= ?
		GROUP BY symbol
		ORDER BY total_pnl DESC`, start)
	if err != nil {
		return nil, fmt.Errorf("memory: pair analytics query: %w", err)
	}
	defer rows.Close()

	var out []PairAnalytics
	for rows.Next() {
		var p PairAnalytics
		if err := rows.Scan(&p.Symbol, &p.Trades, &p.Wins, &p.TotalPnL, &p.AveragePnL); err != nil {
			return nil, err
		}
		if p.Trades > 0 {
			p.WinRate = float64(p.Wins) / float64(p.Trades)
		}
		out = append(out, p)
	}
	if out == nil {
		out = []PairAnalytics{}
	}
	return out, rows.Err()
}
