package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	TotalRequestCount int    `json:"total_request_count"`
	EndTimestamp      string `json:"end_timestamp"`
}

type kiroRTSModelState struct {
	ModelInfo struct {
		ModelName string `json:"model_name"`
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
	if meta.SessionState.RTSModelState != nil {
		model = meta.SessionState.RTSModelState.ModelInfo.ModelName
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, meta.CreatedAt)

	// Derive project name from cwd (last path component).
	project := ""
	if meta.CWD != "" {
		project = filepath.Base(meta.CWD)
	}

	// Parse JSONL file for prompt events.
	jsonlPath := strings.TrimSuffix(jsonPath, ".json") + ".jsonl"
	promptEvents := c.parsePrompts(jsonlPath, sessionID)

	// Create usage records from user_turn_metadatas.
	var records []*storage.UsageRecord
	if meta.SessionState.ConversationMetadata != nil {
		for _, turn := range meta.SessionState.ConversationMetadata.UserTurnMetadatas {
			if turn.TotalRequestCount == 0 {
				continue
			}
			ts, _ := time.Parse(time.RFC3339Nano, turn.EndTimestamp)
			if ts.IsZero() {
				ts = createdAt
			}
			records = append(records, &storage.UsageRecord{
				Source:    "kiro",
				SessionID: sessionID,
				Model:     model,
				Timestamp: ts,
				Project:   project,
			})
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

// parsePrompts reads the JSONL file and extracts prompt timestamps.
func (c *KiroCollector) parsePrompts(jsonlPath, sessionID string) []*storage.PromptEvent {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var events []*storage.PromptEvent
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
		if entry.Kind != "Prompt" {
			continue
		}
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
	}
	return events
}
