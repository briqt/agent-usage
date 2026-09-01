package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOMPCollectorScansExactUsageAndIncrementalRows(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectDir, "session.jsonl")
	ts := "2026-08-01T10:00:00Z"
	content := `{"type":"session","id":"session-1","cwd":"/work/agent-usage","timestamp":"` + ts + `","version":3}
{"type":"model_change","id":"mc1","timestamp":"` + ts + `","model":"xai-oauth/grok-4.5"}
{"type":"message","id":"u1","timestamp":"` + ts + `","message":{"role":"user","content":"hello"}}
{"type":"message","id":"a1","timestamp":"` + ts + `","message":{"role":"assistant","provider":"xai-oauth","model":"grok-4.5","usage":{"input":100,"output":20,"cacheRead":30,"cacheWrite":10,"reasoningTokens":5,"cost":{"total":0.25}}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	collector := NewOMPCollector(db, []string{dir})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "omp", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTokens != 160 || stats.TotalPrompts != 1 || stats.TotalCalls != 1 {
		t.Fatalf("unexpected OMP stats: %+v", stats)
	}
	if stats.TotalCost != 0.25 {
		t.Fatalf("cost = %v, want 0.25", stats.TotalCost)
	}
	sessions, err := db.GetSessions(from, to, "omp", "")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions=%v err=%v", sessions, err)
	}
	if sessions[0].SessionID != "omp:session-1" || sessions[0].Project != "agent-usage" {
		t.Fatalf("unexpected session: %+v", sessions[0])
	}

	// A second scan without appended data must not duplicate usage or prompts.
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	stats, _ = db.GetDashboardStats(from, to, "omp", "")
	if stats.TotalTokens != 160 || stats.TotalPrompts != 1 {
		t.Fatalf("incremental scan duplicated data: %+v", stats)
	}
}
