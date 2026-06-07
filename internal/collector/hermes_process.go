package collector

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
	_ "modernc.org/sqlite"
)

func (c *HermesCollector) processDB(dbPath, project string) error {
	hermesDB, err := sql.Open("sqlite", dbPath+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return fmt.Errorf("open hermes db: %w", err)
	}
	defer hermesDB.Close()

	if err := c.processSessions(hermesDB, project); err != nil {
		return err
	}
	return c.processPromptEvents(hermesDB)
}

func (c *HermesCollector) processSessions(hermesDB *sql.DB, project string) error {
	rows, err := hermesDB.Query(`
		SELECT id, model, started_at,
			COALESCE(input_tokens, 0),
			COALESCE(output_tokens, 0),
			COALESCE(cache_read_tokens, 0),
			COALESCE(cache_write_tokens, 0),
			COALESCE(reasoning_tokens, 0),
			COALESCE(api_call_count, 1)
		FROM sessions
		WHERE model IS NOT NULL
			AND TRIM(model) != ''
			AND (input_tokens + output_tokens + cache_read_tokens + cache_write_tokens) > 0
			AND started_at IS NOT NULL
			AND started_at > 0
	`)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var records []*storage.UsageRecord
	var sessions []*storage.SessionRecord

	for rows.Next() {
		var (
			sessionID  string
			model      string
			startedAt  float64
			input      int64
			output     int64
			cacheRead  int64
			cacheWrite int64
			reasoning  int64
			apiCalls   int
		)
		if err := rows.Scan(&sessionID, &model, &startedAt,
			&input, &output, &cacheRead, &cacheWrite, &reasoning, &apiCalls); err != nil {
			continue
		}

		ts := time.Unix(int64(startedAt), int64((startedAt-float64(int64(startedAt)))*1e9))

		records = append(records, &storage.UsageRecord{
			Source:                   "hermes",
			SessionID:                sessionID,
			Model:                    model,
			Timestamp:                ts,
			Project:                  project,
			InputTokens:              input,
			OutputTokens:             output,
			CacheReadInputTokens:     cacheRead,
			CacheCreationInputTokens: cacheWrite,
			ReasoningOutputTokens:    reasoning,
			APICalls:                 apiCalls,
		})

		sessions = append(sessions, &storage.SessionRecord{
			Source:    "hermes",
			SessionID: sessionID,
			Project:   project,
			StartTime: ts,
			Prompts:   0,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sessions: %w", err)
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert usage: %w", err)
		}
	}
	for _, sess := range sessions {
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert session: %w", err)
		}
	}
	return nil
}

func (c *HermesCollector) processPromptEvents(hermesDB *sql.DB) error {
	rows, err := hermesDB.Query(`
		SELECT session_id, timestamp
		FROM messages
		WHERE role = 'user' AND timestamp IS NOT NULL AND timestamp > 0
		ORDER BY timestamp
	`)
	if err != nil {
		return fmt.Errorf("query prompt events: %w", err)
	}
	defer rows.Close()

	var events []*storage.PromptEvent
	for rows.Next() {
		var (
			sessionID string
			ts        float64
		)
		if err := rows.Scan(&sessionID, &ts); err != nil {
			continue
		}
		events = append(events, &storage.PromptEvent{
			Source:    "hermes",
			SessionID: sessionID,
			Timestamp: time.Unix(int64(ts), int64((ts-float64(int64(ts)))*1e9)),
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate prompt events: %w", err)
	}

	if len(events) > 0 {
		return c.db.InsertPromptBatch(events)
	}
	return nil
}
