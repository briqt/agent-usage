package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createHermesTestDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test hermes db: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			model TEXT,
			started_at REAL NOT NULL,
			ended_at REAL,
			message_count INTEGER DEFAULT 0,
			tool_call_count INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0,
			cache_write_tokens INTEGER DEFAULT 0,
			reasoning_tokens INTEGER DEFAULT 0,
			parent_session_id TEXT,
			title TEXT,
			user_id TEXT,
			api_call_count INTEGER DEFAULT 0,
			billing_provider TEXT,
			billing_base_url TEXT,
			billing_mode TEXT,
			estimated_cost_usd REAL,
			actual_cost_usd REAL,
			cost_status TEXT,
			cost_source TEXT,
			pricing_version TEXT
		);
		CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			timestamp REAL
		);
	`)
	if err != nil {
		t.Fatalf("create hermes schema: %v", err)
	}
}

func TestHermesCollector_BasicScan(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	// Create a global state.db
	hermesDBPath := filepath.Join(dir, "state.db")
	createHermesTestDB(t, hermesDBPath)

	hdb, _ := sql.Open("sqlite", hermesDBPath)
	ts := float64(time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC).Unix())
	hdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens, api_call_count, title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-001", "telegram", "claude-opus-4-6", ts, 5000, 200, 100, 50, 10, 3, "Test session")
	hdb.Exec(`INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		"sess-001", "user", "hello", ts+10)
	hdb.Exec(`INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		"sess-001", "assistant", "hi there", ts+15)
	hdb.Exec(`INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		"sess-001", "user", "thanks", ts+30)
	hdb.Close()

	cx := NewHermesCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "hermes", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "sess-001" {
		t.Errorf("expected session_id sess-001, got %s", sessions[0].SessionID)
	}
	if sessions[0].Source != "hermes" {
		t.Errorf("expected source hermes, got %s", sessions[0].Source)
	}
	if sessions[0].Prompts != 2 {
		t.Errorf("expected 2 prompts, got %d", sessions[0].Prompts)
	}

	stats, err := db.GetDashboardStats(from, to, "hermes", "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 total session, got %d", stats.TotalSessions)
	}
	expectedTokens := int64(5000 + 200 + 100 + 50)
	if stats.TotalTokens != expectedTokens {
		t.Errorf("expected %d total tokens, got %d", expectedTokens, stats.TotalTokens)
	}
}

func TestHermesCollector_ProfileDiscovery(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	// Global state.db
	globalDBPath := filepath.Join(dir, "state.db")
	createHermesTestDB(t, globalDBPath)
	gdb, _ := sql.Open("sqlite", globalDBPath)
	ts1 := float64(time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC).Unix())
	gdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "global-1", "telegram", "claude-opus-4-6", ts1, 1000, 100)
	gdb.Close()

	// Profile state.db
	profileDir := filepath.Join(dir, "profiles", "my-profile")
	os.MkdirAll(profileDir, 0o755)
	profileDBPath := filepath.Join(profileDir, "state.db")
	createHermesTestDB(t, profileDBPath)
	pdb, _ := sql.Open("sqlite", profileDBPath)
	ts2 := float64(time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC).Unix())
	pdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "profile-1", "wecom", "claude-opus-4-6", ts2, 2000, 200)
	pdb.Close()

	cx := NewHermesCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "hermes", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify projects
	projects := map[string]string{}
	for _, s := range sessions {
		projects[s.SessionID] = s.Project
	}
	if projects["global-1"] != "hermes" {
		t.Errorf("expected project 'hermes' for global, got %q", projects["global-1"])
	}
	if projects["profile-1"] != "my-profile" {
		t.Errorf("expected project 'my-profile' for profile, got %q", projects["profile-1"])
	}
}

func TestHermesCollector_SkipsEmptySessions(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	hermesDBPath := filepath.Join(dir, "state.db")
	createHermesTestDB(t, hermesDBPath)
	hdb, _ := sql.Open("sqlite", hermesDBPath)
	ts := float64(time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC).Unix())
	// Session with tokens
	hdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "active-1", "telegram", "claude-opus-4-6", ts, 500, 50)
	// Session with no tokens
	hdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "empty-1", "telegram", "claude-opus-4-6", ts+60, 0, 0)
	// Session with no model
	hdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "nomodel-1", "telegram", nil, ts+120, 100, 10)
	hdb.Close()

	cx := NewHermesCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "hermes", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (only active), got %d", len(sessions))
	}
	if sessions[0].SessionID != "active-1" {
		t.Errorf("expected active-1, got %s", sessions[0].SessionID)
	}
}

func TestHermesCollector_RescanUpdatesTokens(t *testing.T) {
	db := tempDB(t)
	dir := t.TempDir()

	hermesDBPath := filepath.Join(dir, "state.db")
	createHermesTestDB(t, hermesDBPath)
	hdb, _ := sql.Open("sqlite", hermesDBPath)
	ts := float64(time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC).Unix())
	hdb.Exec(`INSERT INTO sessions (id, source, model, started_at, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?)`, "sess-1", "telegram", "claude-opus-4-6", ts, 1000, 100)
	hdb.Close()

	cx := NewHermesCollector(db, []string{dir})
	cx.Scan()

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	stats, _ := db.GetDashboardStats(from, to, "hermes", "")
	if stats.TotalTokens != 1100 {
		t.Fatalf("first scan: expected 1100 tokens, got %d", stats.TotalTokens)
	}

	// Simulate token growth (agent keeps running)
	hdb, _ = sql.Open("sqlite", hermesDBPath)
	hdb.Exec(`UPDATE sessions SET input_tokens = 2000, output_tokens = 200 WHERE id = 'sess-1'`)
	hdb.Close()

	cx.Scan()

	stats, _ = db.GetDashboardStats(from, to, "hermes", "")
	if stats.TotalTokens != 2200 {
		t.Errorf("rescan: expected 2200 tokens (updated), got %d", stats.TotalTokens)
	}
}

func TestHermesCollector_NonExistentPath(t *testing.T) {
	db := tempDB(t)
	cx := NewHermesCollector(db, []string{"/nonexistent/path"})
	if err := cx.Scan(); err != nil {
		t.Errorf("Scan on non-existent path should not error, got: %v", err)
	}
}
