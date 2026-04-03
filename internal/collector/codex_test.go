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
	stats, err := db.GetDashboardStats(from, to)
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens, got %d", stats.TotalTokens)
	}
}
