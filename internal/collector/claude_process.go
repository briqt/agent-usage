package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"devobs/internal/storage"
)

func (c *ClaudeCollector) processFile(path, project string) error {
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

	var sessionID, version, cwd, gitBranch string
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
		var entry claudeEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}
		if entry.SessionID != "" {
			sessionID = entry.SessionID
		}
		if entry.Version != "" {
			version = entry.Version
		}
		if entry.CWD != "" {
			cwd = entry.CWD
		}
		if entry.GitBranch != "" {
			gitBranch = entry.GitBranch
		}

		switch entry.Type {
		case "user":
			prompts++
		case "assistant":
			if entry.Message == nil {
				continue
			}
			var msg claudeMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			if msg.Usage == nil || msg.Usage.CacheCreationInputTokens == nil {
				continue // streaming chunk, skip
			}
			rec := &storage.UsageRecord{
				Source:    "claude",
				SessionID: sessionID,
				Model:     msg.Model,
				Timestamp: ts,
				Project:   project,
				GitBranch: gitBranch,
			}
			if msg.Usage.InputTokens != nil {
				rec.InputTokens = *msg.Usage.InputTokens
			}
			if msg.Usage.OutputTokens != nil {
				rec.OutputTokens = *msg.Usage.OutputTokens
			}
			if msg.Usage.CacheCreationInputTokens != nil {
				rec.CacheCreationInputTokens = *msg.Usage.CacheCreationInputTokens
			}
			if msg.Usage.CacheReadInputTokens != nil {
				rec.CacheReadInputTokens = *msg.Usage.CacheReadInputTokens
			}
			records = append(records, rec)
		}
	}

	if sessionID == "" {
		sessionID = filepath.Base(path)
		sessionID = sessionID[:len(sessionID)-len(filepath.Ext(sessionID))]
	}

	if len(records) > 0 {
		// Fill session ID for records that were parsed before we found it
		for _, r := range records {
			if r.SessionID == "" {
				r.SessionID = sessionID
			}
		}
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert usage: %w", err)
		}
	}

	if prompts > 0 || len(records) > 0 {
		sess := &storage.SessionRecord{
			Source:    "claude",
			SessionID: sessionID,
			Project:   project,
			CWD:       cwd,
			Version:   version,
			GitBranch: gitBranch,
			StartTime: firstTime,
			Prompts:   prompts,
		}
		c.db.UpsertSession(sess)
	}

	return c.db.SetFileState(path, info.Size(), info.Size())
}
