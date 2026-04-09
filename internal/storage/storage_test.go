package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestFileState(t *testing.T) {
	db := tempDB(t)

	size, offset, err := db.GetFileState("/tmp/test.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 0 || offset != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", size, offset)
	}

	if err := db.SetFileState("/tmp/test.jsonl", 1024, 512); err != nil {
		t.Fatalf("SetFileState: %v", err)
	}

	size, offset, err = db.GetFileState("/tmp/test.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 1024 || offset != 512 {
		t.Errorf("expected (1024,512), got (%d,%d)", size, offset)
	}
}

func TestInsertUsageAndDedup(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := &UsageRecord{
		Source:       "claude",
		SessionID:    "sess-1",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  1000,
		OutputTokens: 500,
		Timestamp:    ts,
	}

	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// Insert same record again — should be silently ignored (dedup)
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage duplicate: %v", err)
	}

	// Verify only one record exists
	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 1500 {
		t.Errorf("expected 1500 tokens, got %d", stats.TotalTokens)
	}
}

func TestInsertUsageBatch(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "sess-1", Model: "claude-sonnet-4-20250514", InputTokens: 100, OutputTokens: 50, Timestamp: ts},
		{Source: "claude", SessionID: "sess-1", Model: "claude-sonnet-4-20250514", InputTokens: 200, OutputTokens: 100, Timestamp: ts.Add(time.Second)},
	}

	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens, got %d", stats.TotalTokens)
	}
}

func TestUpsertSession(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	sess := &SessionRecord{
		Source:    "claude",
		SessionID: "sess-1",
		Project:   "myproject",
		CWD:       "/home/user/code",
		Version:   "1.0.0",
		StartTime: ts,
		Prompts:   5,
	}
	if err := db.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Upsert again with updated prompts
	sess2 := &SessionRecord{
		Source:    "claude",
		SessionID: "sess-1",
		Prompts:   3,
		StartTime: ts.Add(time.Hour),
	}
	if err := db.UpsertSession(sess2); err != nil {
		t.Fatalf("UpsertSession update: %v", err)
	}
}

func TestPricing(t *testing.T) {
	db := tempDB(t)

	if err := db.UpsertPricing("claude-sonnet-4-20250514", 0.003, 0.015, 0.001, 0.004); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}

	inp, out, cr, cc, err := db.GetPricing("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("GetPricing: %v", err)
	}
	if inp != 0.003 || out != 0.015 || cr != 0.001 || cc != 0.004 {
		t.Errorf("unexpected pricing: %f %f %f %f", inp, out, cr, cc)
	}

	// Non-existent model returns zeros
	inp, out, cr, cc, err = db.GetPricing("nonexistent")
	if err != nil {
		t.Fatalf("GetPricing nonexistent: %v", err)
	}
	if inp != 0 || out != 0 || cr != 0 || cc != 0 {
		t.Errorf("expected zeros for nonexistent model")
	}

	all, err := db.GetAllPricing()
	if err != nil {
		t.Fatalf("GetAllPricing: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 model, got %d", len(all))
	}
}

func TestRecalcCosts(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Insert a record with zero cost
	rec := &UsageRecord{
		Source:       "claude",
		SessionID:    "sess-1",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0,
		Timestamp:    ts,
	}
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// Set up pricing
	prices := map[string][4]float64{
		"claude-sonnet-4-20250514": {0.003, 0.015, 0.001, 0.004},
	}

	calcFn := func(input, output, cc, cr int64, p [4]float64) float64 {
		return float64(input)*p[0] + float64(output)*p[1]
	}

	if err := db.RecalcCosts(prices, calcFn); err != nil {
		t.Fatalf("RecalcCosts: %v", err)
	}

	// Verify cost was updated
	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	// 1000*0.003 + 500*0.015 = 3.0 + 7.5 = 10.5
	if stats.TotalCost < 10.4 || stats.TotalCost > 10.6 {
		t.Errorf("expected ~10.5, got %f", stats.TotalCost)
	}
}

func TestGetCostByModel(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts},
		{Source: "claude", SessionID: "s1", Model: "model-b", InputTokens: 200, OutputTokens: 100, CostUSD: 3.0, Timestamp: ts.Add(time.Second)},
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts.Add(2 * time.Second)},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	result, err := db.GetCostByModel(from, to, "")
	if err != nil {
		t.Fatalf("GetCostByModel: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 models, got %d", len(result))
	}
	// Ordered by cost DESC: model-b (3.0), model-a (3.0)
	if result[0].Model != "model-a" && result[0].Model != "model-b" {
		t.Errorf("unexpected model: %s", result[0].Model)
	}
}

func TestGetCostOverTime(t *testing.T) {
	db := tempDB(t)
	ts1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 1.0, Timestamp: ts1},
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 2.0, Timestamp: ts2},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts1.Add(-time.Hour)
	to := ts2.Add(time.Hour)
	result, err := db.GetCostOverTime(from, to, "1d", "", 0)
	if err != nil {
		t.Fatalf("GetCostOverTime: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 points, got %d", len(result))
	}
}

func TestGetCostOverTimeWithTimezone(t *testing.T) {
	db := tempDB(t)
	// 2025-01-01 17:00 UTC = 2025-01-02 01:00 UTC+8
	ts := time.Date(2025, 1, 1, 17, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 1.0, Timestamp: ts},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	// UTC+8 local day: 2025-01-02 00:00 ~ 23:59 local = 2025-01-01 16:00 ~ 2025-01-02 15:59 UTC
	from := time.Date(2025, 1, 1, 16, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 2, 15, 59, 59, 0, time.UTC)
	result, err := db.GetCostOverTime(from, to, "1d", "", -480)
	if err != nil {
		t.Fatalf("GetCostOverTime: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	// Should bucket into local date 2025-01-02, not UTC date 2025-01-01
	if result[0].Date != "2025-01-02" {
		t.Errorf("expected date 2025-01-02, got %s", result[0].Date)
	}
}

func TestGetSessions(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	sess := &SessionRecord{
		Source: "claude", SessionID: "sess-1", Project: "proj",
		CWD: "/home", StartTime: ts, Prompts: 3,
	}
	if err := db.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	rec := &UsageRecord{
		Source: "claude", SessionID: "sess-1", Model: "model-a",
		InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts,
	}
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	sessions, err := db.GetSessions(from, to, "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "sess-1" {
		t.Errorf("expected sess-1, got %s", sessions[0].SessionID)
	}
	if sessions[0].TotalCost != 1.5 {
		t.Errorf("expected cost 1.5, got %f", sessions[0].TotalCost)
	}
}

func TestGetDashboardStatsCacheHitRate(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 1000, OutputTokens: 200, CacheReadInputTokens: 600, Timestamp: ts},
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 500, OutputTokens: 100, CacheReadInputTokens: 200, Timestamp: ts.Add(time.Second)},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	// cache_read=800, input_tokens=1500 → 800/1500 ≈ 0.5333
	expected := 800.0 / 1500.0
	if stats.CacheHitRate < expected-0.001 || stats.CacheHitRate > expected+0.001 {
		t.Errorf("expected CacheHitRate ~%f, got %f", expected, stats.CacheHitRate)
	}
}

func TestGetDashboardStatsCacheHitRateZeroInput(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.CacheHitRate != 0 {
		t.Errorf("expected CacheHitRate 0 for empty data, got %f", stats.CacheHitRate)
	}
}

func TestFileStateUpsert(t *testing.T) {
	db := tempDB(t)

	if err := db.SetFileState("/tmp/a.jsonl", 100, 50); err != nil {
		t.Fatalf("SetFileState: %v", err)
	}
	if err := db.SetFileState("/tmp/a.jsonl", 200, 200); err != nil {
		t.Fatalf("SetFileState update: %v", err)
	}

	size, offset, err := db.GetFileState("/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 200 || offset != 200 {
		t.Errorf("expected (200,200), got (%d,%d)", size, offset)
	}
}
