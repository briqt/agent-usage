package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCodexCollector_Scan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	jsonl := `{"timestamp":"` + ts + `","type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/home/user","cli_version":"0.1.0"}}
{"timestamp":"` + ts + `","type":"turn_context","payload":{"model":"o4-mini"}}
{"timestamp":"` + ts + `","type":"response_item","payload":{"role":"user"}}
{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t1","info":{"last_token_usage":{"input_tokens":300,"cached_input_tokens":50,"output_tokens":150,"reasoning_output_tokens":20}}}}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens, got %d", stats.TotalTokens)
	}
}

func TestCodexCollector_IncrementalScanPreservesSessionContext(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(sessionDir, "session.jsonl")
	initial := `{"timestamp":"` + ts1 + `","type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/home/user","cli_version":"0.1.0"}}
{"timestamp":"` + ts1 + `","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"` + ts1 + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t1","info":{"last_token_usage":{"input_tokens":300,"cached_input_tokens":50,"output_tokens":150,"reasoning_output_tokens":20}}}}
`
	if err := os.WriteFile(fpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	incremental := `{"timestamp":"` + ts2 + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t2","info":{"last_token_usage":{"input_tokens":400,"cached_input_tokens":100,"output_tokens":200,"reasoning_output_tokens":30}}}}
`
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(incremental); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "codex")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after incremental scan, got %d", len(sessions))
	}
	if sessions[0].SessionID != "codex-sess-1" {
		t.Fatalf("expected preserved session_id codex-sess-1, got %s", sessions[0].SessionID)
	}

	details, err := db.GetSessionDetail("codex-sess-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 model detail row, got %d", len(details))
	}
	if details[0].Model != "gpt-5.4" {
		t.Fatalf("expected preserved model gpt-5.4, got %q", details[0].Model)
	}
	if details[0].Calls != 2 {
		t.Fatalf("expected 2 calls after incremental scan, got %d", details[0].Calls)
	}
}
