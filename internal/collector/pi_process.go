package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

func (c *PiCollector) processFile(path, project string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	_, lastOffset, scanCtx, err := c.db.GetFileState(path)
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

	var sessionID, cwd, currentModel string
	if lastOffset > 0 && scanCtx != nil {
		sessionID = scanCtx.SessionID
		cwd = scanCtx.CWD
		currentModel = scanCtx.Model
	}
	var records []*storage.UsageRecord
	var promptEvents []*storage.PromptEvent
	var prompts int
	var firstTime time.Time

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry piEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}

		switch entry.Type {
		case "session":
			if entry.ID != "" {
				sessionID = entry.ID
			}
			if entry.CWD != "" {
				cwd = entry.CWD
			}

		case "model_change":
			if entry.ModelID != "" {
				currentModel = entry.ModelID
			}

		case "message":
			if entry.Message == nil {
				continue
			}
			var msg piMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}

			switch msg.Role {
			case "user":
				prompts++
				promptEvents = append(promptEvents, &storage.PromptEvent{
					Source: "pi", SessionID: sessionID, Timestamp: ts,
				})
			case "assistant":
				if msg.Usage == nil {
					continue
				}
				model := msg.Model
				if model == "" {
					model = currentModel
				}
				if model == "delivery-mirror" {
					continue
				}
				rec := &storage.UsageRecord{
					Source:                   "pi",
					SessionID:                sessionID,
					Model:                    model,
					Timestamp:                ts,
					Project:                  project,
					InputTokens:              msg.Usage.Input,
					OutputTokens:             msg.Usage.Output,
					CacheReadInputTokens:     msg.Usage.CacheRead,
					CacheCreationInputTokens: msg.Usage.CacheWrite,
				}
				records = append(records, rec)
			// toolResult messages are not counted as prompts
			}
		}
	}

	if sessionID == "" {
		sessionID = filepath.Base(path)
		sessionID = sessionID[:len(sessionID)-len(filepath.Ext(sessionID))]
	}

	// If we have a CWD from the session entry, derive project from it
	// (more reliable than slug parsing)
	if cwd != "" {
		if derived := filepath.Base(cwd); derived != "" && derived != "." && derived != "/" {
			project = derived
		}
	}

	for _, r := range records {
		if r.SessionID == "" {
			r.SessionID = sessionID
		}
		r.Project = project
	}
	for _, e := range promptEvents {
		if e.SessionID == "" {
			e.SessionID = sessionID
		}
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert pi usage: %w", err)
		}
	}

	if len(promptEvents) > 0 {
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert pi prompts: %w", err)
		}
	}

	if prompts > 0 || len(records) > 0 {
		sess := &storage.SessionRecord{
			Source:    "pi",
			SessionID: sessionID,
			Project:   project,
			CWD:       cwd,
			StartTime: firstTime,
			Prompts:   prompts,
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert pi session: %w", err)
		}
	}

	return c.db.SetFileState(path, info.Size(), info.Size(), &storage.FileScanContext{
		SessionID: sessionID,
		CWD:       cwd,
		Model:     currentModel,
	})
}
