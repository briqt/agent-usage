package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKiroCollector_Scan(t *testing.T) {
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

	sessions, err := db.GetSessions(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 2 {
		t.Errorf("expected 2 prompts, got %d", sessions[0].Prompts)
	}

	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 2 {
		t.Errorf("expected 2 API calls (usage records), got %d", stats.TotalCalls)
	}
	// Verify token estimation is non-zero (input from context% + output from JSONL).
	if stats.TotalTokens == 0 {
		t.Errorf("expected non-zero total tokens from estimation")
	}
}

func TestKiroCollector_TokenEstimation(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	// Turn 0: 10% of 100000 = 10000 input tokens, 2 requests
	// Turn 1: 20% of 100000 = 20000 input tokens, 3 requests
	metaJSON := `{
		"session_id": "kiro-tokens",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 2,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 10.0
					},
					{
						"total_request_count": 3,
						"end_timestamp": "2026-04-28T08:00:00.000Z",
						"context_usage_percentage": 20.0
					}
				]
			},
			"rts_model_state": {
				"model_info": {
					"model_name": "claude-sonnet-4-20250514",
					"context_window_tokens": 100000
				}
			}
		}
	}`

	// "hello world" = 11 chars, all western → ceil(11/4) = 3 tokens
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

	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}

	// Input: turn0=10000, turn1=20000; Output: 3 tokens → total = 30003
	if stats.TotalTokens != 30003 {
		t.Errorf("expected 30003 total tokens, got %d", stats.TotalTokens)
	}
}

func TestKiroCollector_TokenEstimationCJK(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-cjk",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 1,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 5.0
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

	// "你好世界" = 4 CJK chars → ceil((4*2)/4) = 2 tokens
	jsonlContent := `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"你好世界"}]}}
`

	os.WriteFile(filepath.Join(dir, "kiro-cjk.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-cjk.jsonl"), []byte(jsonlContent), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}

	// Input: 5% of 200000 = 10000; Output: 2 → total = 10002
	if stats.TotalTokens != 10002 {
		t.Errorf("expected 10002 total tokens, got %d", stats.TotalTokens)
	}
}

func TestKiroCollector_NullSessionState(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-subagent",
		"cwd": "/tmp",
		"created_at": "2026-04-28T06:56:23.608Z",
		"session_state": null
	}`

	os.WriteFile(filepath.Join(dir, "kiro-subagent.json"), []byte(metaJSON), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for null session_state, got %d", len(sessions))
	}
}

func TestKiroCollector_EmptyJSONL(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-empty",
		"cwd": "/tmp",
		"created_at": "2026-04-28T06:56:23.608Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 1,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 5.0
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

	os.WriteFile(filepath.Join(dir, "kiro-empty.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-empty.jsonl"), []byte(""), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 call, got %d", stats.TotalCalls)
	}
	// 5% of 200000 = 10000 input, 0 output → 10000 total
	if stats.TotalTokens != 10000 {
		t.Errorf("expected 10000 total tokens, got %d", stats.TotalTokens)
	}
}

func TestKiroCollector_IncrementalScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()

	metaV1 := `{"session_id":"kiro-inc","cwd":"/tmp","created_at":"2026-04-28T06:56:23.608Z","session_state":{"conversation_metadata":{"user_turn_metadatas":[{"total_request_count":1,"end_timestamp":"2026-04-28T07:00:00.000Z","context_usage_percentage":5.0}]},"rts_model_state":{"model_info":{"model_name":"claude-sonnet-4-20250514","context_window_tokens":200000}}}}`

	os.WriteFile(filepath.Join(dir, "kiro-inc.json"), []byte(metaV1), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-inc.jsonl"), []byte(""), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	// Second scan with same file — should be skipped.
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 call (no duplicates), got %d", stats.TotalCalls)
	}
}

func TestKiroCollector_MissingPath(t *testing.T) {
	db := tempDB(t)
	kc := NewKiroCollector(db, []string{"/nonexistent/path"})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestKiroCollector_IgnoresNonJSON(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()

	// Create a .lock file and a subdirectory — both should be ignored.
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
		{"hello world", 3},  // 11 western chars → ceil(11/4) = 3
		{"你好世界", 2},         // 4 CJK → ceil(8/4) = 2
		{"hello你好", 3},      // 5 western + 2 CJK → ceil((4+5)/4) = 3
		{"a", 1},            // 1 western → ceil(1/4) = 1
		{"abcd", 1},         // 4 western → ceil(4/4) = 1
		{"abcde", 2},        // 5 western → ceil(5/4) = 2
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestKiroCollector_ToolUseOutputTokens(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-tooluse",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 1,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 1.0
					}
				]
			},
			"rts_model_state": {
				"model_info": {
					"model_name": "claude-sonnet-4-20250514",
					"context_window_tokens": 100000
				}
			}
		}
	}`

	// toolUse input: {"path":"/tmp/file.txt"} = 22 chars → ceil(22/4) = 6 tokens
	jsonlContent := `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"toolUse","data":{"toolUseId":"t1","name":"read","input":{"path":"/tmp/file.txt"}}}]}}
`

	os.WriteFile(filepath.Join(dir, "kiro-tooluse.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-tooluse.jsonl"), []byte(jsonlContent), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}

	// 1% of 100000 = 1000 input + some output from toolUse
	if stats.TotalTokens <= 1000 {
		t.Errorf("expected tokens > 1000 (input + toolUse output), got %d", stats.TotalTokens)
	}
}

func TestKiroCollector_ZeroContextPercentage(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	metaJSON := `{
		"session_id": "kiro-zero",
		"cwd": "/tmp/proj",
		"created_at": "2026-04-28T06:00:00.000Z",
		"session_state": {
			"conversation_metadata": {
				"user_turn_metadatas": [
					{
						"total_request_count": 1,
						"end_timestamp": "2026-04-28T07:00:00.000Z",
						"context_usage_percentage": 0
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

	os.WriteFile(filepath.Join(dir, "kiro-zero.json"), []byte(metaJSON), 0o644)
	os.WriteFile(filepath.Join(dir, "kiro-zero.jsonl"), []byte(""), 0o644)

	kc := NewKiroCollector(db, []string{dir})
	if err := kc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	stats, err := db.GetDashboardStats(from, to, "kiro")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}

	// 0% context → 0 input, no JSONL → 0 output
	if stats.TotalTokens != 0 {
		t.Errorf("expected 0 total tokens for zero context percentage, got %d", stats.TotalTokens)
	}
}
