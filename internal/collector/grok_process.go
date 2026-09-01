package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

const grokUSDTicks = 10_000_000_000.0

type grokUpdateEntry struct {
	Timestamp json.RawMessage `json:"timestamp"`
	Params    struct {
		Meta struct {
			AgentTimestampMS int64 `json:"agentTimestampMs"`
		} `json:"_meta"`
		Update struct {
			SessionUpdate   string     `json:"sessionUpdate"`
			PromptID        string     `json:"prompt_id"`
			Usage           *grokUsage `json:"usage"`
			UsageIncomplete bool       `json:"usageIsIncomplete"`
			CostPartial     bool       `json:"costIsPartial"`
		} `json:"update"`
	} `json:"params"`
}

type grokUsage struct {
	InputTokens         int64                `json:"inputTokens"`
	OutputTokens        int64                `json:"outputTokens"`
	CachedReadTokens    int64                `json:"cachedReadTokens"`
	CacheCreationTokens int64                `json:"cacheCreationTokens"`
	ReasoningTokens     int64                `json:"reasoningTokens"`
	ModelCalls          int                  `json:"modelCalls"`
	CostUSDTicks        int64                `json:"costUsdTicks"`
	UsageIncomplete     bool                 `json:"usageIsIncomplete"`
	CostPartial         bool                 `json:"costIsPartial"`
	ModelUsage          map[string]grokUsage `json:"modelUsage"`
}

type grokSummary struct {
	CurrentModelID string          `json:"current_model_id"`
	CreatedAt      json.RawMessage `json:"created_at"`
	Info           json.RawMessage `json:"info"`
}

func (c *GrokCollector) processFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	_, offset, _, err := c.db.GetFileState(path)
	if err != nil {
		return err
	}
	if info.Size() < offset {
		offset = 0
	}
	if info.Size() == offset {
		return nil
	}

	sessionID, cwd, project, model, startTime := readGrokSummary(path)
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

	var records []*storage.UsageRecord
	var prompts []*storage.PromptEvent
	processedOffset := offset
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		completeLine := readErr == nil || (readErr == io.EOF && len(line) > 0 && json.Valid(line))
		if !completeLine {
			if readErr != nil && readErr != io.EOF {
				return readErr
			}
			break
		}
		processedOffset += int64(len(line))
		var entry grokUpdateEntry
		if json.Unmarshal(line, &entry) == nil && entry.Params.Update.SessionUpdate == "turn_completed" && entry.Params.Update.Usage != nil {
			ts := parseGrokTimestamp(entry.Timestamp, entry.Params.Meta.AgentTimestampMS)
			if startTime.IsZero() || (!ts.IsZero() && ts.Before(startTime)) {
				startTime = ts
			}
			promptID := entry.Params.Update.PromptID
			if promptID == "" {
				promptID = strconv.FormatInt(ts.UnixNano(), 10)
			}
			prompts = append(prompts, &storage.PromptEvent{Source: "grok", SessionID: sessionID, Timestamp: ts})

			usage := entry.Params.Update.Usage
			incomplete := entry.Params.Update.CostPartial || entry.Params.Update.UsageIncomplete || usage.CostPartial || usage.UsageIncomplete
			if len(usage.ModelUsage) == 0 {
				records = append(records, grokRecord(sessionID, project, model, promptID, ts, *usage,
					incomplete))
			} else {
				for modelID, modelUsage := range usage.ModelUsage {
					// Older ledgers only put cost on the top-level total.
					if modelUsage.CostUSDTicks == 0 && len(usage.ModelUsage) == 1 {
						modelUsage.CostUSDTicks = usage.CostUSDTicks
					}
					records = append(records, grokRecord(sessionID, project, modelID, promptID, ts, modelUsage,
						incomplete || modelUsage.CostPartial || modelUsage.UsageIncomplete))
				}
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert grok usage: %w", err)
		}
	}
	if len(prompts) > 0 {
		if err := c.db.InsertPromptBatch(prompts); err != nil {
			return fmt.Errorf("insert grok prompts: %w", err)
		}
	}
	if len(records) > 0 || len(prompts) > 0 {
		if err := c.db.UpsertSession(&storage.SessionRecord{
			Source: "grok", SessionID: sessionID, Project: project, CWD: cwd,
			StartTime: startTime, Prompts: len(prompts),
		}); err != nil {
			return fmt.Errorf("upsert grok session: %w", err)
		}
	}
	return c.db.SetFileState(path, info.Size(), processedOffset, nil)
}

func grokRecord(sessionID, project, model, promptID string, ts time.Time, usage grokUsage, incomplete bool) *storage.UsageRecord {
	nonCached := usage.InputTokens - usage.CachedReadTokens - usage.CacheCreationTokens
	if nonCached < 0 {
		nonCached = 0
	}
	reasoning := usage.ReasoningTokens
	if reasoning > usage.OutputTokens {
		reasoning = usage.OutputTokens
	}
	record := &storage.UsageRecord{
		Source: "grok", Provider: "xai", SessionID: sessionID, RequestID: promptID,
		DedupKey: strings.Join([]string{"grok", sessionID, promptID, model}, ":"), Model: model,
		InputTokens: nonCached, OutputTokens: usage.OutputTokens,
		CacheReadInputTokens:     usage.CachedReadTokens,
		CacheCreationInputTokens: usage.CacheCreationTokens,
		ReasoningOutputTokens:    reasoning, TokenQuality: "exact", Timestamp: ts,
		Project: project, APICalls: usage.ModelCalls,
	}
	if usage.CostUSDTicks > 0 && !incomplete {
		record.NativeCostUSD = float64(usage.CostUSDTicks) / grokUSDTicks
		record.NativeCostKind = "actual"
	}
	return record
}

func readGrokSummary(updatesPath string) (sessionID, cwd, project, model string, created time.Time) {
	sessionDir := filepath.Dir(updatesPath)
	sessionID = "grok:" + filepath.Base(sessionDir)
	encodedCWD := filepath.Base(filepath.Dir(sessionDir))
	if decoded, err := url.PathUnescape(encodedCWD); err == nil {
		cwd = decoded
	}
	if cwd != "" {
		project = filepath.Base(cwd)
	}
	data, err := os.ReadFile(filepath.Join(sessionDir, "summary.json"))
	if err != nil {
		return
	}
	var summary grokSummary
	if json.Unmarshal(data, &summary) != nil {
		return
	}
	model = summary.CurrentModelID
	created = parseGrokTimestamp(summary.CreatedAt, 0)
	var info struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}
	if json.Unmarshal(summary.Info, &info) == nil {
		if info.SessionID != "" {
			sessionID = "grok:" + info.SessionID
		}
		if info.CWD != "" {
			cwd, project = info.CWD, filepath.Base(info.CWD)
		}
	}
	return
}

func parseGrokTimestamp(raw json.RawMessage, fallbackMS int64) time.Time {
	if len(raw) > 0 && string(raw) != "null" {
		var number float64
		if json.Unmarshal(raw, &number) == nil {
			if number > 1e12 {
				return time.UnixMilli(int64(number)).UTC()
			}
			return time.Unix(int64(number), int64((number-float64(int64(number)))*1e9)).UTC()
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
				return ts.UTC()
			}
		}
	}
	if fallbackMS > 0 {
		return time.UnixMilli(fallbackMS).UTC()
	}
	return time.Time{}
}
