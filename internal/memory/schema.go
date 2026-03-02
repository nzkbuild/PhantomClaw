package memory

// SQL statements for all 8 PhantomClaw tables (PRD §13.2).
const schemaSQL = `
-- Core trade record
CREATE TABLE IF NOT EXISTS trades (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol      TEXT    NOT NULL,
    direction   TEXT    NOT NULL,
    entry       REAL    NOT NULL,
    exit        REAL,
    lot         REAL    NOT NULL,
    sl          REAL,
    tp          REAL,
    pnl         REAL,
    session     TEXT,
    timeframe   TEXT,
    llm_reason  TEXT,
    confidence  INTEGER,
    opened_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    closed_at   DATETIME
);

-- Post-trade lessons (written by LLM after close)
CREATE TABLE IF NOT EXISTS lessons (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    trade_id    INTEGER REFERENCES trades(id),
    symbol      TEXT    NOT NULL,
    session     TEXT,
    lesson      TEXT    NOT NULL,
    tags        TEXT,
    weight      REAL    NOT NULL DEFAULT 1.0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Per-pair adaptive strategy state (LRU-ranked)
CREATE TABLE IF NOT EXISTS pair_state (
    symbol          TEXT PRIMARY KEY,
    lru_rank        INTEGER NOT NULL DEFAULT 999,
    bias            TEXT    NOT NULL DEFAULT 'neutral',
    preferred_tf    TEXT,
    win_rate_7d     REAL    NOT NULL DEFAULT 0.0,
    avg_pnl_7d      REAL    NOT NULL DEFAULT 0.0,
    session_scores  TEXT,
    last_traded_at  DATETIME,
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Market data cache with TTL
CREATE TABLE IF NOT EXISTS market_cache (
    key         TEXT PRIMARY KEY,
    value       TEXT    NOT NULL,
    source      TEXT,
    expires_at  DATETIME NOT NULL
);

-- Daily trade diary (append-only)
CREATE TABLE IF NOT EXISTS diary (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    date        TEXT    NOT NULL,
    entry_type  TEXT    NOT NULL,
    content     TEXT    NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Session RAM (resets at 00:00 MYT daily)
CREATE TABLE IF NOT EXISTS session_ram (
    key         TEXT PRIMARY KEY,
    value       TEXT    NOT NULL,
    expires_at  DATETIME NOT NULL
);

-- Strategy version patches
CREATE TABLE IF NOT EXISTS strategy_patches (
    patch_id    TEXT PRIMARY KEY,
    description TEXT    NOT NULL,
    diff        TEXT,
    applied_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    rolled_back INTEGER NOT NULL DEFAULT 0
);

-- Telegram conversation context
CREATE TABLE IF NOT EXISTS conversations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    role        TEXT    NOT NULL,
    content     TEXT    NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Pending bridge decisions (durable queue across restarts)
CREATE TABLE IF NOT EXISTS pending_decisions (
    request_id    TEXT PRIMARY KEY,
    symbol        TEXT    NOT NULL,
    decision_json TEXT    NOT NULL,
    status        TEXT    NOT NULL DEFAULT 'pending', -- pending | delivered | consumed | expired
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at    DATETIME NOT NULL
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_trades_symbol    ON trades(symbol);
CREATE INDEX IF NOT EXISTS idx_trades_opened    ON trades(opened_at);
CREATE INDEX IF NOT EXISTS idx_lessons_trade    ON lessons(trade_id);
CREATE INDEX IF NOT EXISTS idx_lessons_symbol   ON lessons(symbol);
CREATE INDEX IF NOT EXISTS idx_diary_date       ON diary(date);
CREATE INDEX IF NOT EXISTS idx_diary_type       ON diary(entry_type);
CREATE INDEX IF NOT EXISTS idx_cache_expires    ON market_cache(expires_at);
CREATE INDEX IF NOT EXISTS idx_ram_expires      ON session_ram(expires_at);
CREATE INDEX IF NOT EXISTS idx_pending_symbol_status ON pending_decisions(symbol, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_pending_expires       ON pending_decisions(expires_at);
`
