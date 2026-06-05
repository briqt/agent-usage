package collector

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/briqt/agent-usage/internal/storage"
)

type kiroSessionState struct {
	RTSModelState *kiroRTSModelState `json:"rts_model_state"`
}

type kiroRTSModelState struct {
	ModelInfo struct {
		ModelName           string `json:"model_name"`
		ContextWindowTokens int    `json:"context_window_tokens"`
	} `json:"model_info"`
}

type kiroSQLiteConversation struct {
	ConversationID string              `json:"conversation_id"`
	History        []kiroSQLiteTurn    `json:"history"`
	ModelInfo      kiroSQLiteModelInfo `json:"model_info"`
	SessionState   *kiroSessionState   `json:"session_state"`
}

type kiroSQLiteModelInfo struct {
	ModelName           string `json:"model_name"`
	ModelID             string `json:"model_id"`
	ContextWindowTokens int64  `json:"context_window_tokens"`
}

type kiroSQLiteTurn struct {
	RequestMetadata kiroRequestMetadata `json:"request_metadata"`
}

type kiroRequestMetadata struct {
	RequestID               string  `json:"request_id"`
	ContextUsagePercentage  float64 `json:"context_usage_percentage"`
	RequestStartTimestampMS int64   `json:"request_start_timestamp_ms"`
	UserPromptLength        int64   `json:"user_prompt_length"`
	ResponseSize            int64   `json:"response_size"`
	ModelID                 string  `json:"model_id"`
}

func estimateTokensFromLength(length int64) int64 {
	if length <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(length) / 4.0))
}

func requestTimestamp(startMS int64, requestID string) time.Time {
	ts := time.UnixMilli(startMS).UTC()
	if requestID == "" {
		return ts
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(requestID))
	return ts.Add(time.Duration(h.Sum32()%1_000_000) * time.Nanosecond)
}

func (c *KiroCollector) processSQLite(dbPath string) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		return err
	}
	_, lastRequestStartMS, _, err := c.db.GetFileState(dbPath)
	if err != nil {
		return err
	}

	src, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer src.Close()

	rows, err := src.Query(`
		SELECT key, conversation_id, created_at, value
		FROM conversations_v2
		WHERE updated_at >= ?
		ORDER BY updated_at`, lastRequestStartMS)
	if err != nil {
		return fmt.Errorf("query conversations_v2: %w", err)
	}
	defer rows.Close()

	var records []*storage.UsageRecord
	var prompts []*storage.PromptEvent
	sessionPrompts := map[string]int{}
	sessionMeta := map[string]*storage.SessionRecord{}
	maxRequestStartMS := lastRequestStartMS

	for rows.Next() {
		var key, conversationID, raw string
		var createdAtMS int64
		if err := rows.Scan(&key, &conversationID, &createdAtMS, &raw); err != nil {
			return err
		}

		var conv kiroSQLiteConversation
		if err := json.Unmarshal([]byte(raw), &conv); err != nil {
			continue
		}
		if conversationID == "" {
			conversationID = conv.ConversationID
		}
		if conversationID == "" {
			continue
		}

		model := conv.ModelInfo.ModelID
		if model == "" {
			model = conv.ModelInfo.ModelName
		}
		contextWindowTokens := conv.ModelInfo.ContextWindowTokens
		if conv.SessionState != nil && conv.SessionState.RTSModelState != nil {
			if model == "" {
				model = conv.SessionState.RTSModelState.ModelInfo.ModelName
			}
			if contextWindowTokens == 0 {
				contextWindowTokens = int64(conv.SessionState.RTSModelState.ModelInfo.ContextWindowTokens)
			}
		}

		project := ""
		if key != "" {
			project = filepath.Base(key)
		}
		if _, ok := sessionMeta[conversationID]; !ok {
			sessionMeta[conversationID] = &storage.SessionRecord{
				Source:    "kiro",
				SessionID: conversationID,
				Project:   project,
				CWD:       key,
				StartTime: time.UnixMilli(createdAtMS).UTC(),
			}
		}

		for _, turn := range conv.History {
			rm := turn.RequestMetadata
			if rm.RequestStartTimestampMS == 0 || rm.RequestStartTimestampMS <= lastRequestStartMS {
				continue
			}
			ts := requestTimestamp(rm.RequestStartTimestampMS, rm.RequestID)
			if rm.RequestStartTimestampMS > maxRequestStartMS {
				maxRequestStartMS = rm.RequestStartTimestampMS
			}
			recordModel := model
			if rm.ModelID != "" {
				recordModel = rm.ModelID
			}
			var inputTokens int64
			if contextWindowTokens > 0 && rm.ContextUsagePercentage > 0 {
				inputTokens = int64(math.Round(rm.ContextUsagePercentage / 100.0 * float64(contextWindowTokens)))
			}
			if inputTokens == 0 {
				inputTokens = estimateTokensFromLength(rm.UserPromptLength)
			}

			records = append(records, &storage.UsageRecord{
				Source:       "kiro",
				SessionID:    conversationID,
				Model:        recordModel,
				Timestamp:    ts,
				Project:      project,
				InputTokens:  inputTokens,
				OutputTokens: estimateTokensFromLength(rm.ResponseSize),
			})
			prompts = append(prompts, &storage.PromptEvent{
				Source:    "kiro",
				SessionID: conversationID,
				Timestamp: ts,
			})
			sessionPrompts[conversationID]++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert kiro sqlite usage: %w", err)
		}
	}
	if len(prompts) > 0 {
		if err := c.db.InsertPromptBatch(prompts); err != nil {
			return fmt.Errorf("insert kiro sqlite prompts: %w", err)
		}
	}
	for sessionID, sess := range sessionMeta {
		sess.Prompts = sessionPrompts[sessionID]
		if sess.Prompts == 0 {
			continue
		}
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert kiro sqlite session: %w", err)
		}
	}

	return c.db.SetFileState(dbPath, info.Size(), maxRequestStartMS, nil)
}

// --- JSON/JSONL session file processing (legacy format) ---

// kiroMetadata represents the JSON metadata file for a Kiro session.
type kiroMetadata struct {
	SessionID    string                `json:"session_id"`
	CWD          string                `json:"cwd"`
	CreatedAt    string                `json:"created_at"`
	Title        string                `json:"title"`
	SessionState *kiroJSONSessionState `json:"session_state"`
}

type kiroJSONSessionState struct {
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

// estimateTokens estimates the token count for a string using a CJK-aware heuristic:
// CJK characters ≈ 0.5 token each (2 char-units), other characters ≈ 0.25 tokens each (1 char-unit).
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

	project := ""
	if meta.CWD != "" {
		project = filepath.Base(meta.CWD)
	}

	jsonlPath := strings.TrimSuffix(jsonPath, ".json") + ".jsonl"
	promptEvents, totalOutputTokens := c.parseJSONL(jsonlPath, sessionID)

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

			var inputTokens int64
			if contextWindowTokens > 0 && turn.ContextUsagePercentage > 0 {
				inputTokens = int64(math.Round(turn.ContextUsagePercentage / 100.0 * float64(contextWindowTokens)))
			}

			var turnOutput int64
			if totalRequests > 0 && totalOutputTokens > 0 {
				if i == len(turns)-1 {
					turnOutput = totalOutputTokens - outputDistributed
				} else {
					turnOutput = int64(math.Round(float64(totalOutputTokens) * float64(turn.TotalRequestCount) / float64(totalRequests)))
					outputDistributed += turnOutput
				}
			}

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
