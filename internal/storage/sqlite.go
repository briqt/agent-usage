package storage

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type UsageRecord struct {
	ID                       int64
	Source                   string // "claude" or "codex"
	SessionID                string
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	ReasoningOutputTokens    int64
	CostUSD                  float64
	Timestamp                time.Time
	Project                  string
	GitBranch                string
}

type SessionRecord struct {
	ID        int64
	Source    string
	SessionID string
	Project   string
	CWD       string
	Version   string
	GitBranch string
	StartTime time.Time
	Prompts   int
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			timestamp DATETIME NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_records(source);

		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL UNIQUE,
			project TEXT DEFAULT '',
			cwd TEXT DEFAULT '',
			version TEXT DEFAULT '',
			git_branch TEXT DEFAULT '',
			start_time DATETIME,
			prompts INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS file_state (
			path TEXT PRIMARY KEY,
			size INTEGER DEFAULT 0,
			last_offset INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS pricing (
			model TEXT PRIMARY KEY,
			input_cost_per_token REAL DEFAULT 0,
			output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0,
			cache_creation_input_token_cost REAL DEFAULT 0,
			updated_at DATETIME
		);
	`)
	return err
}
