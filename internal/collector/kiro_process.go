package collector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"time"

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
