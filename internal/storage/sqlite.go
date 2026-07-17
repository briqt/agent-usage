package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection with a mutex for safe concurrent access.
type DB struct {
	db *sql.DB
	mu sync.Mutex
}

// UsageRecord represents a single API call's token usage and cost.
type UsageRecord struct {
	ID                       int64
	Source                   string // "claude" or "codex"
	Provider                 string
	SessionID                string
	RequestID                string
	MessageID                string
	DedupKey                 string
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheCreation5mTokens    int64
	CacheCreation1hTokens    int64
	CacheReadInputTokens     int64
	ReasoningOutputTokens    int64
	CostUSD                  float64 // Effective USD cost: reported first, token-priced fallback.
	NativeCostUSD            float64 // Source-returned cost used internally for precedence.
	NativeCostKind           string  // Internal provenance: actual or source_estimate.
	CodexCredits             float64 // Deprecated compatibility field; no longer calculated.
	TokenQuality             string  // exact or estimated
	PriceSource              string
	Speed                    string
	InferenceGeo             string
	Timestamp                time.Time
	Project                  string
	GitBranch                string
	APICalls                 int
}

// SessionRecord represents metadata for a coding agent session.
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

// PromptEvent represents a single user prompt with its timestamp.
type PromptEvent struct {
	Source    string
	SessionID string
	Timestamp time.Time
}

// Open creates or opens a SQLite database at the given path, enables WAL mode,
// and runs schema migrations.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			provider TEXT DEFAULT '',
			session_id TEXT NOT NULL,
			request_id TEXT DEFAULT '',
			message_id TEXT DEFAULT '',
			dedup_key TEXT DEFAULT '',
			model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			cache_creation_5m_tokens INTEGER DEFAULT 0,
			cache_creation_1h_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			native_cost_usd REAL DEFAULT 0,
			native_cost_kind TEXT DEFAULT '',
			codex_credits REAL DEFAULT 0,
			token_quality TEXT DEFAULT 'exact',
			price_source TEXT DEFAULT '',
			speed TEXT DEFAULT '',
			inference_geo TEXT DEFAULT '',
			priced_at DATETIME,
			timestamp DATETIME NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_records(source);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(session_id, model, timestamp, input_tokens, output_tokens);

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

		CREATE TABLE IF NOT EXISTS prompt_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_prompt_timestamp ON prompt_events(timestamp);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_dedup ON prompt_events(session_id, timestamp);

		CREATE TABLE IF NOT EXISTS file_state (
			path TEXT PRIMARY KEY,
			size INTEGER DEFAULT 0,
			last_offset INTEGER DEFAULT 0,
			scan_context TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS pricing (
			model TEXT PRIMARY KEY,
			input_cost_per_token REAL DEFAULT 0,
			output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0,
			cache_creation_input_token_cost REAL DEFAULT 0,
			cache_creation_1h_input_token_cost REAL DEFAULT 0,
			fast_multiplier REAL DEFAULT 1,
			source TEXT DEFAULT 'litellm',
			updated_at DATETIME
		);

		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		);

		DELETE FROM usage_records WHERE model = '<synthetic>';
		DELETE FROM usage_records WHERE model = 'delivery-mirror';
	`)
	if err != nil {
		return err
	}

	// Add scan_context column to file_state for existing DBs (idempotent).
	_, _ = db.Exec("ALTER TABLE file_state ADD COLUMN scan_context TEXT DEFAULT ''")
	// Add api_calls column to usage_records for session-level collectors (idempotent).
	_, _ = db.Exec("ALTER TABLE usage_records ADD COLUMN api_calls INTEGER DEFAULT 1")
	billingAlters := []string{
		"ALTER TABLE usage_records ADD COLUMN provider TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN request_id TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN message_id TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN dedup_key TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN cache_creation_5m_tokens INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN cache_creation_1h_tokens INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN native_cost_usd REAL DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN native_cost_kind TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN codex_credits REAL DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN token_quality TEXT DEFAULT 'exact'",
		"ALTER TABLE usage_records ADD COLUMN price_source TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN speed TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN inference_geo TEXT DEFAULT ''",
		"ALTER TABLE usage_records ADD COLUMN priced_at DATETIME",
		"ALTER TABLE pricing ADD COLUMN cache_creation_1h_input_token_cost REAL DEFAULT 0",
		"ALTER TABLE pricing ADD COLUMN fast_multiplier REAL DEFAULT 1",
		"ALTER TABLE pricing ADD COLUMN source TEXT DEFAULT 'litellm'",
	}
	for _, statement := range billingAlters {
		_, _ = db.Exec(statement)
	}

	// Versioned migrations: each runs once, tracked via meta table.
	migrations := []struct {
		id  string
		sql string
	}{
		{
			"001_fix_opencode_input_tokens", `
				DELETE FROM usage_records WHERE source = 'opencode';
				DELETE FROM file_state WHERE path LIKE '%opencode%';
				DELETE FROM sessions WHERE source = 'opencode';
			`,
		},
		{
			"002_input_tokens_non_overlapping", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
			`,
		},
		{
			"003_prompt_events_rescan", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
				DELETE FROM prompt_events;
			`,
		},
		{
			"004_file_state_scan_context", `
				DELETE FROM meta WHERE key LIKE 'file_scan_context:%';
				DELETE FROM file_state;
			`,
		},
		{
			"005_kiro_sqlite_only_rescan", `
				DELETE FROM usage_records WHERE source = 'kiro';
				DELETE FROM prompt_events WHERE source = 'kiro';
				DELETE FROM sessions WHERE source = 'kiro';
				DELETE FROM file_state WHERE path LIKE '%kiro%';
			`,
		},
		{
			"006_billing_accuracy", `
				DELETE FROM usage_records
				 WHERE source = 'claude'
				   AND input_tokens = 0
				   AND output_tokens = 0
				   AND cache_creation_input_tokens = 0
				   AND cache_read_input_tokens = 0;

				WITH previous AS (
					SELECT id, session_id, model, input_tokens, output_tokens,
					       cache_creation_input_tokens, cache_read_input_tokens,
					       reasoning_output_tokens, timestamp,
					       LAG(timestamp) OVER (
						       PARTITION BY session_id, model, input_tokens, output_tokens,
							    cache_creation_input_tokens, cache_read_input_tokens,
							    reasoning_output_tokens
						       ORDER BY timestamp, id
					       ) AS previous_timestamp
					  FROM usage_records
					 WHERE source = 'claude'
				),
				marked AS (
					SELECT *,
					       CASE
						       WHEN previous_timestamp IS NULL THEN 1
						       WHEN (julianday(substr(timestamp, 1, 19)) - julianday(substr(previous_timestamp, 1, 19))) * 86400.0 > 300.0 THEN 1
						       ELSE 0
					       END AS new_cluster
					  FROM previous
				),
				clustered AS (
					SELECT *,
					       SUM(new_cluster) OVER (
						       PARTITION BY session_id, model, input_tokens, output_tokens,
							    cache_creation_input_tokens, cache_read_input_tokens,
							    reasoning_output_tokens
						       ORDER BY timestamp, id
						       ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
					       ) AS cluster_id
					  FROM marked
				),
				ranked AS (
					SELECT id,
					       ROW_NUMBER() OVER (
						       PARTITION BY session_id, model, input_tokens, output_tokens,
							    cache_creation_input_tokens, cache_read_input_tokens,
							    reasoning_output_tokens, cluster_id
						       ORDER BY timestamp, id
					       ) AS duplicate_rank
					  FROM clustered
				)
				DELETE FROM usage_records
				 WHERE id IN (SELECT id FROM ranked WHERE duplicate_rank > 1);

				UPDATE usage_records SET token_quality = 'estimated' WHERE source = 'kiro';
			`,
		},
		{
			"007_single_cost", `
				UPDATE usage_records SET codex_credits = 0;
				UPDATE usage_records
				SET cost_usd = native_cost_usd,
					price_source = CASE native_cost_kind
						WHEN 'source_estimate' THEN 'source_reported_estimate'
						ELSE 'source_reported'
					END
				WHERE native_cost_usd > 0;
			`,
		},
	}
	for _, m := range migrations {
		var done string
		db.QueryRow("SELECT value FROM meta WHERE key=?", "migration_"+m.id).Scan(&done)
		if done == "done" {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.id, err)
		}
		db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			"migration_"+m.id, "done")
	}
	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup_key ON usage_records(source, dedup_key) WHERE dedup_key != ''"); err != nil {
		return fmt.Errorf("create billing dedup index: %w", err)
	}

	return nil
}
