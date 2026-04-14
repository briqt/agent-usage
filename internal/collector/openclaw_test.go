package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenClawCollector_IncrementalScanPreservesSessionContext(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "main", "sessions")
	os.MkdirAll(sessionDir, 0o755)

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(sessionDir, "oc-sess-1.jsonl")
	initial := `{"type":"session","id":"oc-sess-1","cwd":"/home/user/project","timestamp":"` + ts1 + `","version":1}
{"type":"message","timestamp":"` + ts1 + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input":200,"output":100,"cacheRead":50,"cacheWrite":30}}}
`
	if err := os.WriteFile(fpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewOpenClawCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	incremental := `{"type":"message","timestamp":"` + ts2 + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input":300,"output":150,"cacheRead":60,"cacheWrite":40}}}
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

	sessions, err := db.GetSessions(from, to, "openclaw")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "oc-sess-1" {
		t.Fatalf("expected preserved session_id oc-sess-1, got %s", sessions[0].SessionID)
	}

	details, err := db.GetSessionDetail("oc-sess-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 model detail row, got %d", len(details))
	}
	if details[0].Calls != 2 {
		t.Fatalf("expected 2 calls after incremental scan, got %d", details[0].Calls)
	}
}
