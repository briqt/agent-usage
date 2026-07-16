package collector

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

func TestClaudeCollectorDeduplicatesContentBlocksAndPricesOneHourCache(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := "2026-07-01T00:00:00Z"
	system := `{"type":"system","timestamp":"` + ts + `","sessionId":"sess-billing"}` + "\n"
	assistant := `{"type":"assistant","timestamp":"` + ts + `","sessionId":"sess-billing","requestId":"req-1","message":{"id":"msg-1","role":"assistant","model":"claude-opus-4-7","usage":{"input_tokens":6,"output_tokens":426,"cache_creation_input_tokens":11519,"cache_read_input_tokens":18258,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":11519},"speed":"standard","inference_geo":"global"}}}`
	content := system + strings.Repeat(assistant+"\n", 5)
	if err := os.WriteFile(filepath.Join(projectDir, "session.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	collector := NewClaudeCollector(db, []string{dir})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertModelPricing("claude-opus-4-7", storage.ModelPricing{
		Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6,
		CacheCreation5m: 6.25e-6, CacheCreation1h: 10e-6,
		FastMultiplier: 1, Source: "anthropic_official",
	}); err != nil {
		t.Fatal(err)
	}
	prices, err := db.GetAllModelPricing()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecalcCosts(prices); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "claude", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCalls != 1 {
		t.Fatalf("calls = %d, want 1 for five content-block rows", stats.TotalCalls)
	}
	if stats.TotalTokens != 30209 {
		t.Fatalf("tokens = %d, want 30209", stats.TotalTokens)
	}
	if math.Abs(stats.TotalCost-0.134999) > 1e-9 {
		t.Fatalf("cost = %.9f, want 0.134999", stats.TotalCost)
	}
}
