package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestClaudeCollector_Scan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	jsonl := `{"type":"system","timestamp":"` + ts + `","sessionId":"sess-abc","version":"1.2.3","cwd":"/home/user","gitBranch":"main"}
{"type":"user","timestamp":"` + ts + `"}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":500,"output_tokens":200,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}}
`
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Use wide UTC range to avoid timezone issues with SQLite string comparison
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 850 {
		t.Errorf("expected 850 tokens, got %d", stats.TotalTokens)
	}
}

func TestClaudeCollector_IncrementalScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line1 := `{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	fpath := filepath.Join(projDir, "sess.jsonl")
	if err := os.WriteFile(fpath, []byte(line1), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	line2 := `{"type":"assistant","timestamp":"` + ts2 + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	f.WriteString(line2)
	f.Close()

	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens after incremental scan, got %d", stats.TotalTokens)
	}
}

func TestClaudeCollector_MissingPath(t *testing.T) {
	db := tempDB(t)
	cc := NewClaudeCollector(db, []string{"/nonexistent/path"})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestIsRealUserPrompt(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"string content", `{"content":"hello"}`, true},
		{"text block", `{"content":[{"type":"text","text":"hello"}]}`, true},
		{"tool result", `{"content":[{"type":"tool_result","tool_use_id":"abc","content":"ok"}]}`, false},
		{"mixed with tool_use_id", `{"content":[{"tool_use_id":"abc","type":"tool_result"}]}`, false},
		{"empty message", `{}`, false},
		{"nil message", ``, false},
		{"empty content array", `{"content":[]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRealUserPrompt([]byte(tt.raw))
			if got != tt.want {
				t.Errorf("isRealUserPrompt(%s) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClaudeCollector_PromptCounting(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	// 1 real user prompt + 2 tool results (should count as 1 prompt, not 3)
	jsonl := `{"type":"system","timestamp":"` + ts + `","sessionId":"sess-prompt","version":"1.0","cwd":"/tmp"}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":"please help me"}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"file contents"}]}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","content":"done"}]}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":150,"output_tokens":75,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
`
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 1 {
		t.Errorf("expected 1 prompt (real user message only), got %d", sessions[0].Prompts)
	}
}

func TestCodexCollector_EmptyFile(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	if err := os.WriteFile(filepath.Join(sessionDir, "empty.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}
