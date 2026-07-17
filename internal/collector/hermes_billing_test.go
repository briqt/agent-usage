package collector

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestHermesCollectorClassifiesNativeCosts(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	createHermesTestDB(t, path)
	source, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ts := float64(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Unix())
	_, err = source.Exec(`
		INSERT INTO sessions(id,source,model,started_at,input_tokens,output_tokens,actual_cost_usd,estimated_cost_usd,cost_status,cost_source)
		VALUES ('actual','cli','claude-opus-4-7',?,10,2,0.30,0.40,'actual','provider'),
		       ('estimate','cli','claude-opus-4-7',?,20,3,0,0.20,'estimated','local');
	`, ts, ts+1)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := NewHermesCollector(db, []string{dir}).Scan(); err != nil {
		t.Fatal(err)
	}
	stats, err := db.GetDashboardStats(
		time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "hermes", "")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(stats.TotalCost-0.50) > 1e-12 {
		t.Fatalf("effective cost = %.2f, want source-returned total 0.50", stats.TotalCost)
	}
}

func TestHermesCollectorSupportsLegacySchemaWithoutCostColumns(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	source, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ts := float64(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Unix())
	_, err = source.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY, model TEXT, started_at REAL,
			input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0, cache_write_tokens INTEGER DEFAULT 0,
			reasoning_tokens INTEGER DEFAULT 0, api_call_count INTEGER DEFAULT 1
		);
		CREATE TABLE messages (
			id INTEGER PRIMARY KEY, session_id TEXT, role TEXT, timestamp REAL
		);
		INSERT INTO sessions(id,model,started_at,input_tokens,output_tokens)
		VALUES ('legacy','claude-opus-4-6',?,100,10);
	`, ts)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := NewHermesCollector(db, []string{dir}).Scan(); err != nil {
		t.Fatal(err)
	}
	stats, err := db.GetDashboardStats(
		time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "hermes", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTokens != 110 || stats.TotalCost != 0 {
		t.Fatalf("legacy stats: tokens=%d cost=%f",
			stats.TotalTokens, stats.TotalCost)
	}
}
