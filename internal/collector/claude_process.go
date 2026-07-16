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

func (c *ClaudeCollector) processFile(path, project string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	_, lastOffset, _, err := c.db.GetFileState(path)
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
			if isRealUserPrompt(entry.Message) {
				prompts++
				promptEvents = append(promptEvents, &storage.PromptEvent{
					Source: "claude", SessionID: sessionID, Timestamp: ts,
				})
			}
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
			if msg.Model == "<synthetic>" {
				continue
			}
			rec := &storage.UsageRecord{
				Source:       "claude",
				Provider:     "anthropic",
				SessionID:    sessionID,
				RequestID:    entry.RequestID,
				MessageID:    msg.ID,
				Model:        msg.Model,
				TokenQuality: "exact",
				Timestamp:    ts,
				Project:      project,
				GitBranch:    gitBranch,
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
			if msg.Usage.CacheCreation != nil {
				if msg.Usage.CacheCreation.Ephemeral5mInputTokens != nil {
					rec.CacheCreation5mTokens = *msg.Usage.CacheCreation.Ephemeral5mInputTokens
				}
				if msg.Usage.CacheCreation.Ephemeral1hInputTokens != nil {
					rec.CacheCreation1hTokens = *msg.Usage.CacheCreation.Ephemeral1hInputTokens
				}
			}
			rec.Speed = msg.Usage.Speed
			rec.InferenceGeo = msg.Usage.InferenceGeo
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
			if r.RequestID != "" && r.MessageID != "" {
				r.DedupKey = fmt.Sprintf("%s:%s:%s", r.SessionID, r.RequestID, r.MessageID)
			} else if r.RequestID != "" {
				r.DedupKey = fmt.Sprintf("%s:%s", r.SessionID, r.RequestID)
			} else if r.MessageID != "" {
				r.DedupKey = fmt.Sprintf("%s:%s", r.SessionID, r.MessageID)
			}
		}
		records = deduplicateClaudeRecords(records)
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert usage: %w", err)
		}
	}

	if len(promptEvents) > 0 {
		for _, e := range promptEvents {
			if e.SessionID == "" {
				e.SessionID = sessionID
			}
		}
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert prompts: %w", err)
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
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert claude session: %w", err)
		}
	}

	return c.db.SetFileState(path, info.Size(), info.Size(), nil)
}

func deduplicateClaudeRecords(records []*storage.UsageRecord) []*storage.UsageRecord {
	result := make([]*storage.UsageRecord, 0, len(records))
	positions := make(map[string]int)
	for _, record := range records {
		if record.DedupKey == "" {
			result = append(result, record)
			continue
		}
		if position, ok := positions[record.DedupKey]; ok {
			if claudeRecordScore(record) > claudeRecordScore(result[position]) {
				result[position] = record
			}
			continue
		}
		positions[record.DedupKey] = len(result)
		result = append(result, record)
	}
	return result
}

func claudeRecordScore(record *storage.UsageRecord) int64 {
	score := record.InputTokens + record.OutputTokens + record.CacheCreationInputTokens +
		record.CacheReadInputTokens
	if record.CacheCreation5mTokens+record.CacheCreation1hTokens > 0 {
		score += 1
	}
	if record.Speed != "" {
		score += 1
	}
	if record.InferenceGeo != "" {
		score += 1
	}
	return score
}
