package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPiCollector_BasicScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-wangl-codes-agent-usage--")
	os.MkdirAll(wsDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(wsDir, "2026-05-09T03-14-24-634Z_019e0aba.jsonl")
	content := `{"type":"session","id":"pi-sess-1","cwd":"/home/wangl/codes/agent-usage","timestamp":"` + ts + `","version":3}
{"type":"model_change","id":"mc1","parentId":null,"timestamp":"` + ts + `","provider":"litellm-dev","modelId":"claude-sonnet-4.6"}
{"type":"message","id":"m1","parentId":"mc1","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"hello"}],"timestamp":1778296825316}}
{"type":"message","id":"m2","parentId":"m1","timestamp":"` + ts + `","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"model":"claude-sonnet-4.6","usage":{"input":200,"output":50,"cacheRead":30,"cacheWrite":20,"totalTokens":300},"stopReason":"stop"}}
`
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "pi-sess-1" {
		t.Errorf("expected session_id pi-sess-1, got %s", sessions[0].SessionID)
	}
	if sessions[0].Project != "agent-usage" {
		t.Errorf("expected project agent-usage, got %s", sessions[0].Project)
	}
	if sessions[0].Prompts != 1 {
		t.Errorf("expected 1 prompt, got %d", sessions[0].Prompts)
	}

	details, err := db.GetSessionDetail("pi-sess-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 detail row, got %d", len(details))
	}
	if details[0].InputTokens != 200 {
		t.Errorf("expected input_tokens 200, got %d", details[0].InputTokens)
	}
	if details[0].OutputTokens != 50 {
		t.Errorf("expected output_tokens 50, got %d", details[0].OutputTokens)
	}
	if details[0].CacheRead != 30 {
		t.Errorf("expected cache_read 30, got %d", details[0].CacheRead)
	}
	if details[0].CacheCreate != 20 {
		t.Errorf("expected cache_write 20, got %d", details[0].CacheCreate)
	}
}

func TestPiCollector_IncrementalScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-wangl-codes-myproject--")
	os.MkdirAll(wsDir, 0o755)

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(wsDir, "session1.jsonl")
	initial := `{"type":"session","id":"pi-inc-1","cwd":"/home/wangl/codes/myproject","timestamp":"` + ts1 + `","version":3}
{"type":"model_change","id":"mc1","timestamp":"` + ts1 + `","modelId":"claude-sonnet-4.6"}
{"type":"message","id":"m1","timestamp":"` + ts1 + `","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input":100,"output":50,"cacheRead":10,"cacheWrite":5}}}
`
	if err := os.WriteFile(fpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	// Append more data
	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	incremental := `{"type":"message","id":"m2","timestamp":"` + ts2 + `","message":{"role":"user","content":[{"type":"text","text":"more"}]}}
{"type":"message","id":"m3","timestamp":"` + ts2 + `","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input":200,"output":100,"cacheRead":20,"cacheWrite":10}}}
`
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	f.WriteString(incremental)
	f.Close()

	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "pi-inc-1" {
		t.Errorf("expected session_id pi-inc-1, got %s", sessions[0].SessionID)
	}

	details, err := db.GetSessionDetail("pi-inc-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 model detail row, got %d", len(details))
	}
	if details[0].Calls != 2 {
		t.Errorf("expected 2 calls after incremental scan, got %d", details[0].Calls)
	}
}

func TestPiCollector_ModelChangeTracking(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-user-project--")
	os.MkdirAll(wsDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(wsDir, "model-change.jsonl")
	content := `{"type":"session","id":"pi-mc-1","cwd":"/home/user/project","timestamp":"` + ts + `","version":3}
{"type":"model_change","id":"mc1","timestamp":"` + ts + `","modelId":"claude-sonnet-4.6"}
{"type":"message","id":"m1","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"q1"}]}}
{"type":"message","id":"m2","timestamp":"` + ts + `","message":{"role":"assistant","usage":{"input":100,"output":50,"cacheRead":0,"cacheWrite":0}}}
{"type":"model_change","id":"mc2","timestamp":"` + ts + `","modelId":"claude-opus-4.6"}
{"type":"message","id":"m3","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"q2"}]}}
{"type":"message","id":"m4","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-opus-4.6","usage":{"input":200,"output":100,"cacheRead":0,"cacheWrite":0}}}
`
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	details, err := db.GetSessionDetail("pi-mc-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	// Should have 2 model rows: claude-sonnet-4.6 and claude-opus-4.6
	if len(details) != 2 {
		t.Fatalf("expected 2 model detail rows, got %d", len(details))
	}

	modelCalls := map[string]int{}
	for _, d := range details {
		modelCalls[d.Model] = d.Calls
	}
	if modelCalls["claude-sonnet-4.6"] != 1 {
		t.Errorf("expected 1 call for claude-sonnet-4.6, got %d", modelCalls["claude-sonnet-4.6"])
	}
	if modelCalls["claude-opus-4.6"] != 1 {
		t.Errorf("expected 1 call for claude-opus-4.6, got %d", modelCalls["claude-opus-4.6"])
	}
}

func TestPiCollector_EmptySession(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-user-empty--")
	os.MkdirAll(wsDir, 0o755)

	// Empty file
	fpath := filepath.Join(wsDir, "empty.jsonl")
	if err := os.WriteFile(fpath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty file, got %d", len(sessions))
	}
}

func TestPiCollector_MalformedLines(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-user-malformed--")
	os.MkdirAll(wsDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(wsDir, "malformed.jsonl")
	content := `not valid json at all
{"type":"session","id":"pi-mal-1","cwd":"/tmp","timestamp":"` + ts + `","version":3}
{broken json here
{"type":"message","id":"m1","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"message","id":"m2","timestamp":"` + ts + `","message":{"role":"assistant","model":"gpt-4","usage":{"input":50,"output":25,"cacheRead":0,"cacheWrite":0}}}
`
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session despite malformed lines, got %d", len(sessions))
	}
	if sessions[0].Prompts != 1 {
		t.Errorf("expected 1 prompt, got %d", sessions[0].Prompts)
	}
}

func TestPiCollector_ToolResultNotCountedAsPrompt(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	wsDir := filepath.Join(dir, "--home-user-tools--")
	os.MkdirAll(wsDir, 0o755)

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	ts3 := time.Now().Add(2 * time.Second).UTC().Format(time.RFC3339Nano)
	ts4 := time.Now().Add(3 * time.Second).UTC().Format(time.RFC3339Nano)
	ts5 := time.Now().Add(4 * time.Second).UTC().Format(time.RFC3339Nano)
	ts6 := time.Now().Add(5 * time.Second).UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(wsDir, "tools.jsonl")
	content := `{"type":"session","id":"pi-tool-1","cwd":"/tmp","timestamp":"` + ts1 + `","version":3}
{"type":"message","id":"m1","timestamp":"` + ts2 + `","message":{"role":"user","content":[{"type":"text","text":"run ls"}]}}
{"type":"message","id":"m2","timestamp":"` + ts3 + `","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input":100,"output":50,"cacheRead":0,"cacheWrite":0}}}
{"type":"message","id":"m3","timestamp":"` + ts4 + `","message":{"role":"toolResult","toolCallId":"toolu_123","toolName":"bash","content":[{"type":"text","text":"file1\nfile2"}],"isError":false}}
{"type":"message","id":"m4","timestamp":"` + ts5 + `","message":{"role":"user","content":[{"type":"text","text":"thanks"}]}}
{"type":"message","id":"m5","timestamp":"` + ts6 + `","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input":150,"output":60,"cacheRead":0,"cacheWrite":0}}}
`
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// 2 user messages, 0 toolResult counted
	if sessions[0].Prompts != 2 {
		t.Errorf("expected 2 prompts (toolResult excluded), got %d", sessions[0].Prompts)
	}
}

func TestPiCollector_MultipleWorkspaces(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	ws1 := filepath.Join(dir, "--home-user-codes-projectA--")
	ws2 := filepath.Join(dir, "--home-user-codes-projectB--")
	os.MkdirAll(ws1, 0o755)
	os.MkdirAll(ws2, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)

	f1 := filepath.Join(ws1, "s1.jsonl")
	c1 := `{"type":"session","id":"pi-ws-1","cwd":"/home/user/codes/projectA","timestamp":"` + ts + `","version":3}
{"type":"message","id":"m1","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"message","id":"m2","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input":100,"output":50,"cacheRead":0,"cacheWrite":0}}}
`
	f2 := filepath.Join(ws2, "s2.jsonl")
	c2 := `{"type":"session","id":"pi-ws-2","cwd":"/home/user/codes/projectB","timestamp":"` + ts + `","version":3}
{"type":"message","id":"m1","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}
{"type":"message","id":"m2","timestamp":"` + ts + `","message":{"role":"assistant","model":"gpt-4o","usage":{"input":80,"output":40,"cacheRead":0,"cacheWrite":0}}}
`
	os.WriteFile(f1, []byte(c1), 0o644)
	os.WriteFile(f2, []byte(c2), 0o644)

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "pi", "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions from 2 workspaces, got %d", len(sessions))
	}

	projects := map[string]bool{}
	for _, s := range sessions {
		projects[s.Project] = true
	}
	if !projects["projectA"] {
		t.Errorf("expected project projectA to be present")
	}
	if !projects["projectB"] {
		t.Errorf("expected project projectB to be present")
	}
}

func TestPiCollector_MissingPath(t *testing.T) {
	db := tempDB(t)

	cx := NewPiCollector(db, []string{"/nonexistent/path/that/does/not/exist"})
	// Should not error, just log and skip
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan should not error on missing path: %v", err)
	}
}

func TestDeriveProjectFromSlug(t *testing.T) {
	tests := []struct {
		slug     string
		expected string
	}{
		{"--home-wangl-codes-agent-usage--", "usage"},
		{"--home-wangl-.openclaw-workspace--", "workspace"},
		{"--home-user-project--", "project"},
		{"--home-user-codes-my-app--", "app"},
		{"--home-user-src-big-project--", "project"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		got := deriveProjectFromSlug(tt.slug)
		if got != tt.expected {
			t.Errorf("deriveProjectFromSlug(%q) = %q, want %q", tt.slug, got, tt.expected)
		}
	}
}
