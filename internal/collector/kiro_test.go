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
						"end_timestamp": "2026-04-28T07:00:00.000Z"
					},
					{
						"total_request_count": 5,
						"end_timestamp": "2026-04-28T07:10:00.000Z"
					}
				]
			},
			"rts_model_state": {
				"model_info": {
					"model_name": "claude-sonnet-4-20250514"
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
					{"total_request_count": 1, "end_timestamp": "2026-04-28T07:00:00.000Z"}
				]
			},
			"rts_model_state": {"model_info": {"model_name": "claude-sonnet-4-20250514"}}
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
}

func TestKiroCollector_IncrementalScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()

	metaV1 := `{"session_id":"kiro-inc","cwd":"/tmp","created_at":"2026-04-28T06:56:23.608Z","session_state":{"conversation_metadata":{"user_turn_metadatas":[{"total_request_count":1,"end_timestamp":"2026-04-28T07:00:00.000Z"}]},"rts_model_state":{"model_info":{"model_name":"claude-sonnet-4-20250514"}}}}`

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
