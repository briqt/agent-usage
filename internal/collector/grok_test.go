package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGrokCollectorNormalizesCacheAndUsesReportedCost(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "%2Fwork%2Fagent-usage", "session-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	summary := `{"current_model_id":"grok-4.5-build","created_at":"2026-08-01T10:00:00Z","info":{"session_id":"session-1","cwd":"/work/agent-usage"}}`
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := `{"timestamp":1785578400,"method":"_x.ai/session/update","params":{"sessionId":"session-1","update":{"sessionUpdate":"turn_completed","prompt_id":"prompt-1","usage":{"inputTokens":1000,"outputTokens":100,"cachedReadTokens":600,"cacheCreationTokens":100,"reasoningTokens":40,"modelCalls":3,"costUsdTicks":1250000000,"modelUsage":{"grok-4.5-build":{"inputTokens":1000,"outputTokens":100,"cachedReadTokens":600,"cacheCreationTokens":100,"reasoningTokens":40,"modelCalls":3,"costUsdTicks":1250000000}}}}}}
`
	path := filepath.Join(sessionDir, "updates.jsonl")
	if err := os.WriteFile(path, []byte(updates), 0o644); err != nil {
		t.Fatal(err)
	}
	collector := NewGrokCollector(db, []string{dir})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "grok", "")
	if err != nil {
		t.Fatal(err)
	}
	// non-cached 300 + cache read 600 + cache create 100 + output 100
	if stats.TotalTokens != 1100 || stats.TotalOutputTokens != 100 {
		t.Fatalf("unexpected Grok token stats: %+v", stats)
	}
	if stats.TotalCalls != 3 || stats.TotalPrompts != 1 || stats.TotalCost != 0.125 {
		t.Fatalf("unexpected Grok accounting: %+v", stats)
	}
	details, err := db.GetSessionDetail("grok:session-1")
	if err != nil || len(details) != 1 {
		t.Fatalf("details=%v err=%v", details, err)
	}
	if details[0].InputTokens != 300 || details[0].CacheRead != 600 || details[0].CacheCreate != 100 {
		t.Fatalf("unexpected Grok detail: %+v", details[0])
	}

	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	stats, _ = db.GetDashboardStats(from, to, "grok", "")
	if stats.TotalTokens != 1100 || stats.TotalPrompts != 1 {
		t.Fatalf("incremental scan duplicated data: %+v", stats)
	}
}

func TestGrokCollectorHandlesVeryLargeJSONLLines(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "%2Ftmp", "large")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	largeToolEvent := `{"params":{"update":{"sessionUpdate":"tool_call_update","content":"` + strings.Repeat("x", 11*1024*1024) + `"}}}` + "\n"
	completed := `{"timestamp":"2026-08-01T10:00:00Z","params":{"update":{"sessionUpdate":"turn_completed","prompt_id":"p","usage":{"inputTokens":10,"outputTokens":2,"modelCalls":1,"modelUsage":{"grok-4.5":{"inputTokens":10,"outputTokens":2,"modelCalls":1}}}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "updates.jsonl"), []byte(largeToolEvent+completed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewGrokCollector(db, []string{dir}).Scan(); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "grok", "")
	if stats.TotalTokens != 12 || stats.TotalPrompts != 1 {
		t.Fatalf("large line prevented later usage parsing: %+v", stats)
	}
}

func TestGrokCollectorDoesNotTrustPartialCost(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "%2Ftmp", "partial")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	updates := `{"timestamp":"2026-08-01T10:00:00Z","params":{"update":{"sessionUpdate":"turn_completed","prompt_id":"p","costIsPartial":true,"usage":{"inputTokens":10,"outputTokens":2,"modelCalls":1,"costUsdTicks":9000000000,"modelUsage":{"grok-4.5":{"inputTokens":10,"outputTokens":2,"modelCalls":1,"costUsdTicks":9000000000}}}}}}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "updates.jsonl"), []byte(updates), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewGrokCollector(db, []string{dir}).Scan(); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "grok", "")
	if stats.TotalCost != 0 {
		t.Fatalf("partial reported cost must not be trusted, got %v", stats.TotalCost)
	}
}
