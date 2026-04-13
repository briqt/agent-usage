package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

// CodexCollector scans Codex CLI session JSONL files and extracts usage records.
type CodexCollector struct {
	db    *storage.DB
	paths []string
}

// NewCodexCollector creates a CodexCollector that scans the given base paths.
func NewCodexCollector(db *storage.DB, paths []string) *CodexCollector {
	return &CodexCollector{db: db, paths: paths}
}

type codexEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID         string `json:"id"`
	CWD        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
}

type codexTurnContext struct {
	Model string `json:"model"`
}

type codexEventPayload struct {
	Type   string          `json:"type"`
	TurnID string          `json:"turn_id"`
	Info   *codexTokenInfo `json:"info"`
}

type codexTokenInfo struct {
	LastTokenUsage *codexTokenUsage `json:"last_token_usage"`
}

type codexTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

type codexResponseItem struct {
	Role string `json:"role"`
	Type string `json:"type"`
}

// Scan walks all configured paths and processes new JSONL data from Codex CLI sessions.
func (c *CodexCollector) Scan() error {
	for _, basePath := range c.paths {
		err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if err := c.processFile(path); err != nil {
				log.Printf("codex: error processing %s: %v", path, err)
			}
			return nil
		})
		if err != nil {
			log.Printf("codex: cannot walk %s: %v", basePath, err)
		}
	}
	return nil
}

func (c *CodexCollector) processFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	_, lastOffset, err := c.db.GetFileState(path)
	if err != nil {
		return err
	}
	if info.Size() <= lastOffset {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if lastOffset > 0 {
		if _, err := f.Seek(lastOffset, io.SeekStart); err != nil {
			return err
		}
	}

	var sessionID, cwd, version, model string
	if lastOffset > 0 {
		ctx, err := c.db.GetFileScanContext(path)
		if err != nil {
			return fmt.Errorf("get codex scan context: %w", err)
		}
		sessionID = ctx.SessionID
		cwd = ctx.CWD
		version = ctx.Version
		model = ctx.Model
	}
	var records []*storage.UsageRecord
	var prompts int
	var firstTime time.Time

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry codexEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}

		switch entry.Type {
		case "session_meta":
			var meta codexSessionMeta
			if err := json.Unmarshal(entry.Payload, &meta); err != nil {
				continue
			}
			sessionID = meta.ID
			cwd = meta.CWD
			version = meta.CLIVersion

		case "turn_context":
			var tc codexTurnContext
			if err := json.Unmarshal(entry.Payload, &tc); err != nil {
				continue
			}
			if tc.Model != "" {
				model = tc.Model
			}

		case "response_item":
			var ri codexResponseItem
			if err := json.Unmarshal(entry.Payload, &ri); err != nil {
				continue
			}
			if ri.Role == "user" && ri.Type != "function_call_output" {
				prompts++
			}

		case "event_msg":
			var ep codexEventPayload
			if err := json.Unmarshal(entry.Payload, &ep); err != nil {
				continue
			}
			if ep.Type == "token_count" && ep.Info != nil && ep.Info.LastTokenUsage != nil {
				u := ep.Info.LastTokenUsage
				rec := &storage.UsageRecord{
					Source:                "codex",
					SessionID:             sessionID,
					Model:                 model,
					InputTokens:           u.InputTokens - u.CachedInputTokens,
					OutputTokens:          u.OutputTokens,
					CacheReadInputTokens:  u.CachedInputTokens,
					ReasoningOutputTokens: u.ReasoningOutputTokens,
					Timestamp:             ts,
				}
				records = append(records, rec)
			}
		}
	}

	if sessionID == "" {
		base := filepath.Base(path)
		sessionID = base[:len(base)-len(filepath.Ext(base))]
	}

	for _, r := range records {
		if r.SessionID == "" {
			r.SessionID = sessionID
		}
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert codex usage: %w", err)
		}
	}

	if prompts > 0 || len(records) > 0 {
		sess := &storage.SessionRecord{
			Source:    "codex",
			SessionID: sessionID,
			CWD:       cwd,
			Version:   version,
			StartTime: firstTime,
			Prompts:   prompts,
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert codex session: %w", err)
		}
	}

	if err := c.db.SetFileScanContext(path, &storage.FileScanContext{
		SessionID: sessionID,
		CWD:       cwd,
		Version:   version,
		Model:     model,
	}); err != nil {
		return fmt.Errorf("set codex scan context: %w", err)
	}

	return c.db.SetFileState(path, info.Size(), info.Size())
}
