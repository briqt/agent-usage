package collector

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
	_ "modernc.org/sqlite"
)

// processDB re-syncs one Hermes database and returns the new prompt watermark.
//
// Sessions are fully replaced within this database's project scope, because
// Hermes accumulates tokens on the session row in place and the usage_records
// dedup index covers the token columns — a grown session would otherwise be
// stored as an additional row instead of replacing the previous one. Prompt
// events, by contrast, are append-only and resume from lastRowID.
func (c *HermesCollector) processDB(dbPath, project string, lastRowID int64) (int64, error) {
	hermesDB, err := sql.Open("sqlite", dbPath+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return 0, fmt.Errorf("open hermes db: %w", err)
	}
	defer hermesDB.Close()

	if err := c.db.DeleteBySourceProject("hermes", project); err != nil {
		return 0, fmt.Errorf("clear project %q: %w", project, err)
	}
	if err := c.processSessions(hermesDB, project); err != nil {
		return 0, err
	}
	return c.processPromptEvents(hermesDB, lastRowID)
}

func (c *HermesCollector) processSessions(hermesDB *sql.DB, project string) error {
	columns, err := sqliteTableColumns(hermesDB, "sessions")
	if err != nil {
		return fmt.Errorf("inspect sessions schema: %w", err)
	}
	expression := func(column, fallback string) string {
		if columns[column] {
			return "COALESCE(" + column + ", " + fallback + ")"
		}
		return fallback
	}
	costSelect := fmt.Sprintf("%s, %s, %s, %s",
		expression("actual_cost_usd", "0"),
		expression("estimated_cost_usd", "0"),
		expression("cost_status", "''"),
		expression("cost_source", "''"),
	)
	rows, err := hermesDB.Query(fmt.Sprintf(`
		SELECT id, model, started_at,
			COALESCE(input_tokens, 0),
			COALESCE(output_tokens, 0),
			COALESCE(cache_read_tokens, 0),
			COALESCE(cache_write_tokens, 0),
			COALESCE(reasoning_tokens, 0),
			COALESCE(api_call_count, 1),
			%s
		FROM sessions
		WHERE model IS NOT NULL
			AND TRIM(model) != ''
			AND (input_tokens + output_tokens + cache_read_tokens + cache_write_tokens) > 0
			AND started_at IS NOT NULL
			AND started_at > 0
	`, costSelect))
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var records []*storage.UsageRecord
	var sessions []*storage.SessionRecord

	for rows.Next() {
		var (
			sessionID                 string
			model                     string
			startedAt                 float64
			input                     int64
			output                    int64
			cacheRead                 int64
			cacheWrite                int64
			reasoning                 int64
			apiCalls                  int
			actualCost, estimatedCost float64
			costStatus, costSource    string
		)
		if err := rows.Scan(&sessionID, &model, &startedAt,
			&input, &output, &cacheRead, &cacheWrite, &reasoning, &apiCalls,
			&actualCost, &estimatedCost, &costStatus, &costSource); err != nil {
			continue
		}

		ts := time.Unix(int64(startedAt), int64((startedAt-float64(int64(startedAt)))*1e9))

		record := &storage.UsageRecord{
			Source:                   "hermes",
			SessionID:                sessionID,
			Model:                    model,
			TokenQuality:             "exact",
			Timestamp:                ts,
			Project:                  project,
			InputTokens:              input,
			OutputTokens:             output,
			CacheReadInputTokens:     cacheRead,
			CacheCreationInputTokens: cacheWrite,
			ReasoningOutputTokens:    reasoning,
			APICalls:                 apiCalls,
		}
		if actualCost > 0 {
			record.NativeCostUSD = actualCost
			record.NativeCostKind = "actual"
		} else if estimatedCost > 0 {
			record.NativeCostUSD = estimatedCost
			record.NativeCostKind = "source_estimate"
		}
		_, _ = costStatus, costSource
		records = append(records, record)

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

func sqliteTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// processPromptEvents appends user prompts with a rowid above lastRowID and
// returns the new watermark.
//
// messages.id is INTEGER PRIMARY KEY AUTOINCREMENT — a rowid alias on an
// append-only table — so `id > ?` is a B-tree range scan touching only new rows.
// The previous unbounded form planned as `SCAN messages` plus
// `USE TEMP B-TREE FOR ORDER BY`: a full scan of a table whose rows carry
// message content and reasoning traces (250MB in one real profile), plus an
// external sort, every scan interval, to retrieve a few hundred prompts. Ordering
// is not needed — the rows are inserted, not streamed in order.
//
// The watermark comes from MAX(id) rather than from the returned rows, so rows
// excluded by the role/timestamp filter are stepped over instead of being
// re-examined forever. It is read before the range query and used as the upper
// bound, so rows appended mid-scan are left for the next pass rather than skipped.
func (c *HermesCollector) processPromptEvents(hermesDB *sql.DB, lastRowID int64) (int64, error) {
	var maxRowID int64
	if err := hermesDB.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM messages`).Scan(&maxRowID); err != nil {
		return 0, fmt.Errorf("query prompt watermark: %w", err)
	}
	if maxRowID <= lastRowID {
		return lastRowID, nil
	}

	rows, err := hermesDB.Query(`
		SELECT session_id, timestamp
		FROM messages
		WHERE id > ? AND id <= ?
			AND role = 'user' AND timestamp IS NOT NULL AND timestamp > 0
	`, lastRowID, maxRowID)
	if err != nil {
		return 0, fmt.Errorf("query prompt events: %w", err)
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
		return 0, fmt.Errorf("iterate prompt events: %w", err)
	}

	if len(events) > 0 {
		if err := c.db.InsertPromptBatch(events); err != nil {
			return 0, err
		}
	}
	return maxRowID, nil
}
