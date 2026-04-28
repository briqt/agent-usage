package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/briqt/agent-usage/internal/storage"
)

// kiroMetadata represents the JSON metadata file for a Kiro session.
type kiroMetadata struct {
	SessionID    string            `json:"session_id"`
	CWD          string            `json:"cwd"`
	CreatedAt    string            `json:"created_at"`
	Title        string            `json:"title"`
	SessionState *kiroSessionState `json:"session_state"`
}

type kiroSessionState struct {
	Version              string                    `json:"version"`
	ConversationMetadata *kiroConversationMetadata `json:"conversation_metadata"`
	RTSModelState        *kiroRTSModelState        `json:"rts_model_state"`
}

type kiroConversationMetadata struct {
	UserTurnMetadatas []kiroUserTurnMetadata `json:"user_turn_metadatas"`
}

type kiroUserTurnMetadata struct {
	TotalRequestCount      int     `json:"total_request_count"`
	EndTimestamp           string  `json:"end_timestamp"`
	ContextUsagePercentage float64 `json:"context_usage_percentage"`
}

type kiroRTSModelState struct {
	ModelInfo struct {
		ModelName           string `json:"model_name"`
		ContextWindowTokens int    `json:"context_window_tokens"`
	} `json:"model_info"`
}

// kiroJSONLEntry represents a single line in a Kiro JSONL conversation file.
type kiroJSONLEntry struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type kiroPromptData struct {
	Meta struct {
		Timestamp float64 `json:"timestamp"`
	} `json:"meta"`
}

type kiroAssistantData struct {
	Content []kiroContentBlock `json:"content"`
}

type kiroContentBlock struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type kiroToolUseData struct {
	Input json.RawMessage `json:"input"`
}

// estimateTokens estimates the token count for a string using a simple heuristic:
// CJK characters ≈ 1 token each, other characters ≈ 0.25 tokens each.
func estimateTokens(s string) int64 {
	var cjk, other int
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Hangul, r) {
			cjk++
		} else {
			other++
		}
	}
	return int64(math.Ceil(float64(cjk*2+other) / 4.0))
}

func (c *KiroCollector) processSession(jsonPath string) error {
	info, err := os.Stat(jsonPath)
	if err != nil {
		return err
	}
	_, lastOffset, _, err := c.db.GetFileState(jsonPath)
	if err != nil {
		return err
	}
	// Use file size as change indicator — if unchanged, skip.
	if info.Size() <= lastOffset {
		return nil
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var meta kiroMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse metadata: %w", err)
	}

	// Skip sessions with null session_state (subagent sessions).
	if meta.SessionState == nil {
		return c.db.SetFileState(jsonPath, info.Size(), info.Size(), nil)
	}

	sessionID := meta.SessionID
	if sessionID == "" {
		base := filepath.Base(jsonPath)
		sessionID = strings.TrimSuffix(base, filepath.Ext(base))
	}

	model := ""
	contextWindowTokens := 0
	if meta.SessionState.RTSModelState != nil {
		model = meta.SessionState.RTSModelState.ModelInfo.ModelName
		contextWindowTokens = meta.SessionState.RTSModelState.ModelInfo.ContextWindowTokens
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, meta.CreatedAt)

	// Derive project name from cwd (last path component).
	project := ""
	if meta.CWD != "" {
		project = filepath.Base(meta.CWD)
	}

	// Parse JSONL file for prompt events and output token estimation.
	jsonlPath := strings.TrimSuffix(jsonPath, ".json") + ".jsonl"
	promptEvents, totalOutputTokens := c.parseJSONL(jsonlPath, sessionID)

	// Create usage records from user_turn_metadatas.
	// Each turn may contain multiple API requests (total_request_count).
	// We generate one UsageRecord per request so that total_calls reflects actual API calls.
	var records []*storage.UsageRecord
	if meta.SessionState.ConversationMetadata != nil {
		turns := meta.SessionState.ConversationMetadata.UserTurnMetadatas
		totalRequests := 0
		for _, turn := range turns {
			totalRequests += turn.TotalRequestCount
		}

		var outputDistributed int64
		for i, turn := range turns {
			if turn.TotalRequestCount == 0 {
				continue
			}
			ts, _ := time.Parse(time.RFC3339Nano, turn.EndTimestamp)
			if ts.IsZero() {
				ts = createdAt
			}

			// Estimate input tokens from context usage percentage.
			var inputTokens int64
			if contextWindowTokens > 0 && turn.ContextUsagePercentage > 0 {
				inputTokens = int64(math.Round(turn.ContextUsagePercentage / 100.0 * float64(contextWindowTokens)))
			}

			// Distribute output tokens proportionally across turns by request count.
			var turnOutput int64
			if totalRequests > 0 && totalOutputTokens > 0 {
				if i == len(turns)-1 {
					turnOutput = totalOutputTokens - outputDistributed
				} else {
					turnOutput = int64(math.Round(float64(totalOutputTokens) * float64(turn.TotalRequestCount) / float64(totalRequests)))
					outputDistributed += turnOutput
				}
			}

			// Generate one record per API request within this turn.
			// Input tokens are the same for each request (full context sent each time).
			// Output tokens are split evenly across requests.
			reqCount := turn.TotalRequestCount
			for r := 0; r < reqCount; r++ {
				var outPerReq int64
				if r == reqCount-1 {
					outPerReq = turnOutput - (turnOutput/int64(reqCount))*int64(reqCount-1)
				} else {
					outPerReq = turnOutput / int64(reqCount)
				}
				records = append(records, &storage.UsageRecord{
					Source:       "kiro",
					SessionID:    sessionID,
					Model:        model,
					Timestamp:    ts.Add(time.Duration(r) * time.Millisecond),
					Project:      project,
					InputTokens:  inputTokens,
					OutputTokens: outPerReq,
				})
			}
		}
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert kiro usage: %w", err)
		}
	}

	if len(promptEvents) > 0 {
		if err := c.db.InsertPromptBatch(promptEvents); err != nil {
			return fmt.Errorf("insert kiro prompts: %w", err)
		}
	}

	if len(records) > 0 || len(promptEvents) > 0 {
		sess := &storage.SessionRecord{
			Source:    "kiro",
			SessionID: sessionID,
			Project:   project,
			CWD:       meta.CWD,
			Version:   meta.SessionState.Version,
			StartTime: createdAt,
			Prompts:   len(promptEvents),
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert kiro session: %w", err)
		}
	}

	return c.db.SetFileState(jsonPath, info.Size(), info.Size(), nil)
}

// parseJSONL reads the JSONL file and extracts prompt timestamps and estimates
// total output tokens from AssistantMessage content.
func (c *KiroCollector) parseJSONL(jsonlPath, sessionID string) ([]*storage.PromptEvent, int64) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	var events []*storage.PromptEvent
	var totalOutputTokens int64
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry kiroJSONLEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		switch entry.Kind {
		case "Prompt":
			var pd kiroPromptData
			if err := json.Unmarshal(entry.Data, &pd); err != nil {
				continue
			}
			if pd.Meta.Timestamp == 0 {
				continue
			}
			ts := time.Unix(int64(pd.Meta.Timestamp), int64((pd.Meta.Timestamp-float64(int64(pd.Meta.Timestamp)))*1e9))
			events = append(events, &storage.PromptEvent{
				Source:    "kiro",
				SessionID: sessionID,
				Timestamp: ts.UTC(),
			})
		case "AssistantMessage":
			var ad kiroAssistantData
			if err := json.Unmarshal(entry.Data, &ad); err != nil {
				continue
			}
			for _, block := range ad.Content {
				switch block.Kind {
				case "text":
					var text string
					if err := json.Unmarshal(block.Data, &text); err == nil {
						totalOutputTokens += estimateTokens(text)
					}
				case "toolUse":
					var tu kiroToolUseData
					if err := json.Unmarshal(block.Data, &tu); err == nil && tu.Input != nil {
						totalOutputTokens += estimateTokens(string(tu.Input))
					}
				}
			}
		}
	}
	return events, totalOutputTokens
}
