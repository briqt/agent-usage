package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createKiroSQLite(t *testing.T, sqlitePath string) {
	t.Helper()

	src, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { src.Close() })

	_, err = src.Exec(`
		CREATE TABLE conversations_v2 (
			key TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (key, conversation_id)
		)`)
	if err != nil {
		t.Fatalf("create conversations_v2: %v", err)
	}

	value := `{
		"conversation_id": "conv-sqlite",
		"model_info": {
			"model_name": "claude-opus-4.6",
			"model_id": "claude-opus-4.6",
			"context_window_tokens": 1000000
		},
		"history": [
			{
				"request_metadata": {
					"request_id": "req-1",
					"request_start_timestamp_ms": 1780462801000,
					"context_usage_percentage": 1.5,
					"user_prompt_length": 80,
					"response_size": 120,
					"model_id": "claude-opus-4.6"
				}
			},
			{
				"request_metadata": {
					"request_id": "req-2",
					"request_start_timestamp_ms": 1780462801000,
					"context_usage_percentage": 2.0,
					"user_prompt_length": 100,
					"response_size": 160,
					"model_id": "claude-opus-4.6"
				}
			},
			{
				"request_metadata": {
					"request_id": "req-3",
					"request_start_timestamp_ms": 1780462801000,
					"context_usage_percentage": 0,
					"user_prompt_length": 40,
					"response_size": 40,
					"model_id": "claude-opus-4.6"
				}
			}
		]
	}`
	_, err = src.Exec(`INSERT INTO conversations_v2(key, conversation_id, value, created_at, updated_at)
		VALUES(?,?,?,?,?)`, "/tmp/sqlite-proj", "conv-sqlite", value, int64(1780462800000), int64(1780462805000))
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
}

func TestKiroCollector_SQLiteConversationsV2(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	sqlitePath := filepath.Join(dir, "data.sqlite3")
	createKiroSQLite(t, sqlitePath)

	kc := NewKiroCollector(db, []string{sqlitePath})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	stats, err := db.GetDashboardStats(from, to, "kiro", "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 3 {
		t.Errorf("expected 3 API calls from same-millisecond request_id values, got %d", stats.TotalCalls)
	}
	if stats.TotalPrompts != 3 {
		t.Errorf("expected 3 prompt events, got %d", stats.TotalPrompts)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 session, got %d", stats.TotalSessions)
	}
	if stats.TotalTokens == 0 {
		t.Errorf("expected non-zero token estimate")
	}
}

func TestKiroCollector_SQLiteDirectoryPath(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	createKiroSQLite(t, filepath.Join(dir, "data.sqlite3"))

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "kiro", "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 3 {
		t.Errorf("expected 3 API calls, got %d", stats.TotalCalls)
	}
}

func TestKiroCollector_IgnoresLegacyJSONDirectory(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "legacy.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write legacy json: %v", err)
	}

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "kiro", "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 calls from legacy JSON directory, got %d", stats.TotalCalls)
	}
}

func TestKiroCollector_MissingPath(t *testing.T) {
	db := tempDB(t)
	kc := NewKiroCollector(db, []string{"/nonexistent/path"})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}
