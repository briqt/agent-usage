package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

func (c *OMPCollector) processFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	_, offset, ctx, err := c.db.GetFileState(path)
	if err != nil {
		return err
	}
	// OMP normally appends, but a rewritten/truncated transcript must be rescanned.
	if info.Size() < offset {
		offset, ctx = 0, nil
	}
	if info.Size() == offset {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	var sessionID, cwd, currentModel string
	if ctx != nil {
		sessionID, cwd, currentModel = ctx.SessionID, ctx.CWD, ctx.Model
	}
	var records []*storage.UsageRecord
	var prompts []*storage.PromptEvent
	var firstTime time.Time

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var entry ompEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if firstTime.IsZero() && !ts.IsZero() {
			firstTime = ts
		}
		switch entry.Type {
		case "session":
			if entry.ID != "" {
				sessionID = ompSessionID(entry.ID)
			}
			if entry.CWD != "" {
				cwd = entry.CWD
			}
		case "model_change":
			if entry.Model != "" {
				currentModel = strings.TrimPrefix(entry.Model, "xai-oauth/")
			}
		case "message":
			var msg ompMessage
			if json.Unmarshal(entry.Message, &msg) != nil {
				continue
			}
			if msg.Role == "user" {
				prompts = append(prompts, &storage.PromptEvent{Source: "omp", SessionID: sessionID, Timestamp: ts})
				continue
			}
			if msg.Role != "assistant" || msg.Usage == nil {
				continue
			}
			model := msg.Model
			if model == "" {
				model = currentModel
			}
			reasoning := msg.Usage.ReasoningTokens
			if reasoning > msg.Usage.Output {
				reasoning = msg.Usage.Output
			}
			record := &storage.UsageRecord{
				Source: "omp", Provider: msg.Provider, SessionID: sessionID,
				MessageID: entry.ID, Model: model,
				InputTokens: msg.Usage.Input, OutputTokens: msg.Usage.Output,
				CacheReadInputTokens:     msg.Usage.CacheRead,
				CacheCreationInputTokens: msg.Usage.CacheWrite,
				ReasoningOutputTokens:    reasoning, TokenQuality: "exact", Timestamp: ts,
			}
			if msg.Usage.Cost.Total > 0 {
				record.NativeCostUSD = msg.Usage.Cost.Total
				record.NativeCostKind = "source_estimate"
			}
			records = append(records, record)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if sessionID == "" {
		sessionID = ompSessionID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	project := filepath.Base(cwd)
	if project == "." || project == string(filepath.Separator) {
		project = ""
	}
	for _, record := range records {
		if record.SessionID == "" {
			record.SessionID = sessionID
		}
		record.Project = project
		record.DedupKey = strings.Join([]string{"omp", record.SessionID, record.MessageID}, ":")
	}
	for _, prompt := range prompts {
		if prompt.SessionID == "" {
			prompt.SessionID = sessionID
		}
	}
	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert omp usage: %w", err)
		}
	}
	if len(prompts) > 0 {
		if err := c.db.InsertPromptBatch(prompts); err != nil {
			return fmt.Errorf("insert omp prompts: %w", err)
		}
	}
	if len(records) > 0 || len(prompts) > 0 {
		if err := c.db.UpsertSession(&storage.SessionRecord{
			Source: "omp", SessionID: sessionID, Project: project, CWD: cwd,
			StartTime: firstTime, Prompts: len(prompts),
		}); err != nil {
			return fmt.Errorf("upsert omp session: %w", err)
		}
	}
	return c.db.SetFileState(path, info.Size(), info.Size(), &storage.FileScanContext{
		SessionID: sessionID, CWD: cwd, Model: currentModel,
	})
}

func ompSessionID(id string) string {
	if strings.HasPrefix(id, "omp:") {
		return id
	}
	return "omp:" + id
}
