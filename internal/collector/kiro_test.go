package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- SQLite source tests ---

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
		t.Errorf("expected 3 API calls, got %d", stats.TotalCalls)
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

// --- JSON/JSONL source tests ---

func TestKiroCollector_JSONScan(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-sess-001",
		"cwd": "/home/user/myproject",
		"created_at": "2026-04-28T06:56:23.608Z",
		"title": "test session",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 3,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 7.247
					},
					{
						"total_request_count": 5,
						"end_timestamp": "2026-04-28T07:10:00.000Z",
						"context_usage_percentage": 7.2911
					}
				]
			},
			"rts_model_state": {
				"model_info": {
					"model_name": "claude-sonnet-4-20250514",
					"context_window_tokens": 200000
				}
			}
		}
	}`

	jsonlContent := `{"version":"v1","kind":"Prompt","data":{"meta":{"timestamp":1745823383},"content":[{"kind":"text","data":"hello"}]}}
{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"hi there"}]}}
{"version":"v1","kind":"Prompt","data":{"meta":{"timestamp":1745823400},"content":[{"kind":"text","data":"help me"}]}}
`

	os.WriteFile(filepath.Join(dir, "kiro-sess-001.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-sess-001.jsonl"), []byte(jsonlContent), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "kiro", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 2 {
		t.Errorf("expected 2 prompts, got %d", sessions[0].Prompts)
	}

	stats, err := db.GetDashboardStats(from, to, "kiro", "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 8 {
		t.Errorf("expected 8 API calls (3+5 requests), got %d", stats.TotalCalls)
	}
	if stats.TotalTokens == 0 {
		t.Errorf("expected non-zero total tokens")
	}
}

func TestKiroCollector_JSONTokenEstimation(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-tokens",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{"total_request_count": 2, "end_timestamp": "2026-04-28T07:00:00.000Z", "context_usage_percentage": 10.0},
					{"total_request_count": 3, "end_timestamp": "2026-04-28T08:00:00.000Z", "context_usage_percentage": 20.0}
				]
			},
			"rts_model_state": {"model_info": {"model_name": "claude-sonnet-4-20250514", "context_window_tokens": 100000}}
		}
	}`
	jsonlContent := `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"hello world"}]}}
`

	os.WriteFile(filepath.Join(dir, "kiro-tokens.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-tokens.jsonl"), []byte(jsonlContent), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "kiro", "")
	// Input: turn0=10000×2reqs, turn1=20000×3reqs; Output: 3 tokens → total = 80003
	if stats.TotalTokens != 80003 {
		t.Errorf("expected 80003 total tokens, got %d", stats.TotalTokens)
	}
	if stats.TotalCalls != 5 {
		t.Errorf("expected 5 API calls, got %d", stats.TotalCalls)
	}
}

func TestKiroCollector_JSONTokenEstimationCJK(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-cjk",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [{"total_request_count": 1, "end_timestamp": "2026-04-28T07:00:00.000Z", "context_usage_percentage": 5.0}]
			},
			"rts_model_state": {"model_info": {"model_name": "claude-sonnet-4-20250514", "context_window_tokens": 200000}}
		}
	}`
	jsonlContent := `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"你好世界"}]}}
`

	os.WriteFile(filepath.Join(dir, "kiro-cjk.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-cjk.jsonl"), []byte(jsonlContent), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	kc.Scan()

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "kiro", "")
	// 5% of 200000 = 10000 input; "你好世界" = 4 CJK → ceil(8/4) = 2 output → 10002
	if stats.TotalTokens != 10002 {
		t.Errorf("expected 10002 total tokens, got %d", stats.TotalTokens)
	}
}

func TestKiroCollector_JSONNullSessionState(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "kiro-subagent.json"), []byte(`{
		"session_id": "kiro-subagent", "cwd": "/tmp", "created_at": "2026-04-28T06:56:23.608Z", "session_state": null
	}`), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	kc.Scan()

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, _ := db.GetSessions(from, to, "kiro", "")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for null session_state, got %d", len(sessions))
	}
}

func TestKiroCollector_JSONIncrementalScan(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaV1 := `{"session_id":"kiro-inc","cwd":"/tmp","created_at":"2026-04-28T06:56:23.608Z","session_state":{"conversation_metadata":{"user_turn_metadatas":[{"total_request_count":1,"end_timestamp":"2026-04-28T07:00:00.000Z","context_usage_percentage":5.0}]},"rts_model_state":{"model_info":{"model_name":"claude-sonnet-4-20250514","context_window_tokens":200000}}}}`
	os.WriteFile(filepath.Join(dir, "kiro-inc.json"), []byte(metaV1), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-inc.jsonl"), []byte(""), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	kc.Scan()
	kc.Scan()

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "kiro", "")
	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 call (no duplicates), got %d", stats.TotalCalls)
	}
}

func TestKiroCollector_JSONIgnoresNonJSON(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "something.lock"), []byte("lock"), 0o644)
	os.MkdirAll(filepath.Join(dir, "tasks"), 0o755)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"", 0},
		{"hello world", 3},
		{"你好世界", 2},
		{"hello你好", 3},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// --- Dual-source coexistence test ---

func TestKiroCollector_DualSource(t *testing.T) {
	db := tempDB(t)

	// SQLite source
	sqliteDir := t.TempDir()
	createKiroSQLite(t, filepath.Join(sqliteDir, "data.sqlite3"))

	// JSON source
	jsonDir := t.TempDir()
	metaJSON := `{
		"session_id": "kiro-json-dual",
		"cwd": "/home/user/proj",
		"created_at": "2026-05-01T10:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [{"total_request_count": 2, "end_timestamp": "2026-05-01T10:05:00.000Z", "context_usage_percentage": 3.0}]
			},
			"rts_model_state": {"model_info": {"model_name": "claude-sonnet-4-20250514", "context_window_tokens": 200000}}
		}
	}`
	os.WriteFile(filepath.Join(jsonDir, "kiro-json-dual.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(jsonDir, "kiro-json-dual.jsonl"), []byte(""), 0o644)

	kc := NewKiroCollector(db, []string{sqliteDir, jsonDir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "kiro", "")
	// SQLite: 3 calls + JSON: 2 calls = 5 total
	if stats.TotalCalls != 5 {
		t.Errorf("expected 5 total calls from dual sources, got %d", stats.TotalCalls)
	}
	if stats.TotalSessions != 2 {
		t.Errorf("expected 2 sessions from dual sources, got %d", stats.TotalSessions)
	}
}

func TestKiroCollector_MissingPath(t *testing.T) {
	db := tempDB(t)
	kc := NewKiroCollector(db, []string{"/nonexistent/path"})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}
