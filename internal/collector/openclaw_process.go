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

func (c *OpenClawCollector) processFile(path, agentID string) error {
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

	var sessionID, cwd string
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
		var entry openclawEntry
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

		case "message":
			if entry.Message == nil {
				continue
			}
			var msg openclawMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}

			switch msg.Role {
			case "user":
				if !hasToolResultBlock(msg.Content) {
					prompts++
				}
			case "assistant":
				if msg.Usage == nil {
					continue
				}
				if msg.Model == "delivery-mirror" {
					continue
				}
				rec := &storage.UsageRecord{
					Source:                   "openclaw",
					SessionID:                sessionID,
					Model:                    msg.Model,
					Timestamp:                ts,
					Project:                  agentID,
					InputTokens:              msg.Usage.Input,
					OutputTokens:             msg.Usage.Output,
					CacheReadInputTokens:     msg.Usage.CacheRead,
					CacheCreationInputTokens: msg.Usage.CacheWrite,
				}
				records = append(records, rec)
			}
		}
	}

	if sessionID == "" {
		sessionID = filepath.Base(path)
		sessionID = sessionID[:len(sessionID)-len(filepath.Ext(sessionID))]
	}

	for _, r := range records {
		if r.SessionID == "" {
			r.SessionID = sessionID
		}
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert openclaw usage: %w", err)
		}
	}

	if prompts > 0 || len(records) > 0 {
		sess := &storage.SessionRecord{
			Source:    "openclaw",
			SessionID: sessionID,
			Project:   agentID,
			CWD:       cwd,
			StartTime: firstTime,
			Prompts:   prompts,
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert openclaw session: %w", err)
		}
	}

	return c.db.SetFileState(path, info.Size(), info.Size())
}
