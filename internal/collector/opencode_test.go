package collector

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

func TestOpenCodeCollectorPreservesProviderAndNativeCost(t *testing.T) {
	db := tempDB(t)
	path := filepath.Join(t.TempDir(), "opencode.db")
	source, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Exec(`
		CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT);
		CREATE TABLE message (data TEXT, session_id TEXT, time_created INTEGER);
		INSERT INTO session(id,directory) VALUES ('s1','/workspace/project');
		INSERT INTO message(data,session_id,time_created) VALUES (
			'{"role":"assistant","modelID":"gpt-test","providerID":"openai","cost":0.42,"tokens":{"input":10,"output":2,"reasoning":1,"cache":{"read":3,"write":0}},"time":{"created":1782864000000}}',
			's1',1782864000000
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	collector := NewOpenCodeCollector(db, []string{path})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	prices := map[string]storage.ModelPricing{
		"gpt-test":        {Input: 9e-6, Output: 9e-6, Source: "generic"},
		"openai/gpt-test": {Input: 1e-6, Output: 2e-6, CacheRead: 0.5e-6, Source: "provider"},
	}
	if err := db.RecalcCosts(prices); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "opencode", "")
	if err != nil {
		t.Fatal(err)
	}
	const wantAPI = 15.5e-6
	if math.Abs(stats.TotalCost-wantAPI) > 1e-12 {
		t.Fatalf("provider-qualified API estimate = %.12f, want %.12f", stats.TotalCost, wantAPI)
	}
	if math.Abs(stats.ActualCostUSD-0.42) > 1e-12 {
		t.Fatalf("native actual cost = %.2f, want 0.42", stats.ActualCostUSD)
	}
}
