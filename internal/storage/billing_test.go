package storage

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestCalculateTokenCostClaudeOneHourCache(t *testing.T) {
	record := billingRecord{
		source: "claude", model: "claude-opus-4-7",
		input: 6, output: 426, cacheRead: 18258, cacheCreate: 11519,
		cache1h: 11519,
	}
	price := ModelPricing{
		Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6,
		CacheCreation5m: 6.25e-6, CacheCreation1h: 10e-6,
		FastMultiplier: 1,
	}
	got := calculateTokenCost(record, price)
	const want = 0.134999
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("official-equivalent cost = %.9f, want %.9f", got, want)
	}
}

func TestRecalcCostsPrefersSourceReturnedCost(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	err := db.InsertUsage(&UsageRecord{
		Source: "opencode", Provider: "openai", SessionID: "s", Model: "gpt-5.5",
		InputTokens: 100, OutputTokens: 20, CostUSD: 999,
		NativeCostUSD: 1.23, NativeCostKind: "actual", Timestamp: ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]ModelPricing{
		"openai/gpt-5.5": {Input: 2e-6, Output: 10e-6, Source: "test"},
	}
	if err := db.RecalcCosts(prices); err != nil {
		t.Fatal(err)
	}
	stats, err := db.GetDashboardStats(ts.Add(-time.Hour), ts.Add(time.Hour), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCost != 1.23 {
		t.Fatalf("effective cost = %.2f, want returned cost 1.23", stats.TotalCost)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"api_estimated_cost_usd", "actual_cost_usd", "source_estimated_cost_usd", "codex_credits"} {
		if _, exists := payload[key]; exists {
			t.Fatalf("dashboard stats still expose removed cost field %q", key)
		}
	}
}

func TestCodexUsesTokenPricing(t *testing.T) {
	record := billingRecord{
		source: "codex", model: "gpt-5.5", input: 100, cacheRead: 50, output: 20,
	}
	price := ModelPricing{Input: 2e-6, CacheRead: 0.5e-6, Output: 10e-6}
	const want = 0.000425
	if got := calculateTokenCost(record, price); math.Abs(got-want) > 1e-12 {
		t.Fatalf("Codex token cost = %.9f, want %.9f", got, want)
	}
}

func TestMatchModelPricingDoesNotUseSubstring(t *testing.T) {
	prices := map[string]ModelPricing{
		"reseller/something-gpt-5-special": {Input: 1},
	}
	if _, ok := matchModelPricing("", "gpt-5", prices); ok {
		t.Fatal("substring-only model match must remain unpriced")
	}
	prices["openai/gpt-5"] = ModelPricing{Input: 2}
	if p, ok := matchModelPricing("openai", "gpt-5", prices); !ok || p.Input != 2 {
		t.Fatal("provider-qualified exact model did not match")
	}
}

func TestDedupKeyUniqueAcrossDifferentTimestamps(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		err := db.InsertUsage(&UsageRecord{
			Source: "claude", SessionID: "s", Model: "claude-opus-4-7",
			DedupKey: "s:req:msg", InputTokens: 6, OutputTokens: 426,
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	stats, err := db.GetDashboardStats(base.Add(-time.Hour), base.Add(time.Hour), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCalls != 1 {
		t.Fatalf("deduped calls = %d, want 1", stats.TotalCalls)
	}
}

func TestBillingMigrationPreservesAndDeduplicatesClaudeHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	records := []*UsageRecord{
		{Source: "claude", SessionID: "claude", Model: "model", InputTokens: 1, Timestamp: ts},
		{Source: "claude", SessionID: "claude", Model: "model", InputTokens: 1, Timestamp: ts.Add(time.Second)},
		{Source: "claude", SessionID: "claude", Model: "model", InputTokens: 1, Timestamp: ts.Add(6 * time.Minute)},
		{Source: "claude", SessionID: "zero", Model: "model", Timestamp: ts},
		{Source: "kiro", SessionID: "kiro", Model: "model", TokenQuality: "exact", InputTokens: 1, Timestamp: ts},
	}
	for _, record := range records {
		if err := db.InsertUsage(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertSession(&SessionRecord{
		Source: "claude", SessionID: "historical", StartTime: ts, Prompts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertPromptBatch([]*PromptEvent{{
		Source: "claude", SessionID: "historical", Timestamp: ts,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFileState("/sessions/claude/historical.jsonl", 100, 100, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("DELETE FROM meta WHERE key='migration_006_billing_accuracy'"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var claudeCount int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM usage_records WHERE source='claude'").Scan(&claudeCount); err != nil {
		t.Fatal(err)
	}
	if claudeCount != 2 {
		var timestamps string
		_ = db.db.QueryRow("SELECT GROUP_CONCAT(timestamp, '|') FROM usage_records WHERE source='claude'").Scan(&timestamps)
		t.Fatalf("claude rows after migration = %d, want 2; retained timestamps: %s", claudeCount, timestamps)
	}
	for table, want := range map[string]int{"sessions": 1, "prompt_events": 1, "file_state": 1} {
		var got int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s rows after migration = %d, want %d", table, got, want)
		}
	}
	var quality string
	if err := db.db.QueryRow("SELECT token_quality FROM usage_records WHERE source='kiro'").Scan(&quality); err != nil {
		t.Fatal(err)
	}
	if quality != "estimated" {

		t.Fatalf("kiro token quality = %q, want estimated", quality)
	}
}
func TestSingleCostMigrationClearsCreditsPreservesPricingAndPromotesNativeCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "single-cost.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "codex", SessionID: "s", Model: "gpt-5.5", InputTokens: 1, Timestamp: ts,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "opencode", SessionID: "native", Model: "gpt-5.5",
		InputTokens: 10, CostUSD: 999, NativeCostUSD: 1.23,
		NativeCostKind: "actual", Timestamp: ts.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET codex_credits=42, priced_at=? WHERE source='codex';
		UPDATE usage_records SET cost_usd=999, price_source='litellm', priced_at=? WHERE source='opencode';
		DELETE FROM meta WHERE key='migration_007_single_cost'`, ts, ts); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var credits float64
	var priced int
	if err := db.db.QueryRow(`SELECT codex_credits, priced_at IS NOT NULL FROM usage_records WHERE source='codex'`).Scan(&credits, &priced); err != nil {
		t.Fatal(err)
	}
	if credits != 0 || priced != 1 {
		t.Fatalf("single-cost migration: credits=%f priced=%d, want 0 and 1", credits, priced)
	}
	var cost float64
	var priceSource string
	if err := db.db.QueryRow(`SELECT cost_usd, price_source FROM usage_records WHERE source='opencode'`).Scan(&cost, &priceSource); err != nil {
		t.Fatal(err)
	}
	if cost != 1.23 || priceSource != "source_reported" {
		t.Fatalf("native migration: cost=%f source=%q, want 1.23 and source_reported", cost, priceSource)
	}
}

func TestMatchModelPricingPrefersProviderAndIsDeterministic(t *testing.T) {
	prices := map[string]ModelPricing{
		"gpt-5.5":         {Input: 9, Source: "generic"},
		"openai/gpt-5.5":  {Input: 2, Source: "provider"},
		"claude-opus-4.7": {Input: 3, Source: "dot"},
		"claude-opus-4-7": {Input: 4, Source: "dash"},
	}
	p, ok := matchModelPricing("openai", "gpt-5.5", prices)
	if !ok || p.Source != "provider" {
		t.Fatalf("provider-qualified rate = %#v, %v", p, ok)
	}
	for i := 0; i < 100; i++ {
		p, ok = matchModelPricing("", "claude-opus-4.7", prices)
		if !ok || p.Source != "dot" {
			t.Fatalf("iteration %d returned nondeterministic rate %#v", i, p)
		}
	}
}

func TestOpenUpgradesLegacyBillingSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL,
			session_id TEXT NOT NULL, model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0, cost_usd REAL DEFAULT 0,
			timestamp DATETIME NOT NULL, project TEXT DEFAULT '', git_branch TEXT DEFAULT ''
		);
		CREATE TABLE pricing (
			model TEXT PRIMARY KEY, input_cost_per_token REAL DEFAULT 0,
			output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0,
			cache_creation_input_token_cost REAL DEFAULT 0, updated_at DATETIME
		);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT DEFAULT '');
		INSERT INTO meta(key,value) VALUES
			('migration_001_fix_opencode_input_tokens','done'),
			('migration_002_input_tokens_non_overlapping','done'),
			('migration_003_prompt_events_rescan','done'),
			('migration_004_file_state_scan_context','done'),
			('migration_005_kiro_sqlite_only_rescan','done');
		INSERT INTO usage_records(source,session_id,model,input_tokens,timestamp)
			VALUES ('claude','c','claude-opus-4-7',1,'2026-07-01T00:00:00Z'),
			       ('kiro','k','kiro-model',1,'2026-07-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer db.Close()
	var claudeCount int
	var kiroQuality string
	if err := db.db.QueryRow("SELECT COUNT(*) FROM usage_records WHERE source='claude'").Scan(&claudeCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow("SELECT token_quality FROM usage_records WHERE source='kiro'").Scan(&kiroQuality); err != nil {
		t.Fatal(err)
	}
	if claudeCount != 1 || kiroQuality != "estimated" {
		t.Fatalf("migration result: claude=%d kiro_quality=%q", claudeCount, kiroQuality)
	}
}
