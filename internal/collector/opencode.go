package collector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

// OpenCodeCollector scans OpenCode's SQLite database and extracts usage records.
type OpenCodeCollector struct {
	db    *storage.DB
	paths []string // paths to opencode.db files
}

// NewOpenCodeCollector creates an OpenCodeCollector that reads the given db paths.
func NewOpenCodeCollector(db *storage.DB, paths []string) *OpenCodeCollector {
	return &OpenCodeCollector{db: db, paths: paths}
}

type opencodeMessageData struct {
	Role       string         `json:"role"`
	ModelID    string         `json:"modelID"`
	ProviderID string         `json:"providerID"`
	Cost       float64        `json:"cost"`
	Tokens     opencodeTokens `json:"tokens"`
	Time       opencodeTime   `json:"time"`
	Path       opencodePath   `json:"path"`
	Agent      string         `json:"agent"`
}

type opencodeTokens struct {
	Input     int64        `json:"input"`
	Output    int64        `json:"output"`
	Reasoning int64        `json:"reasoning"`
	Cache     opencodeCache `json:"cache"`
}

type opencodeCache struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
}

type opencodeTime struct {
	Created   int64 `json:"created"`
	Completed int64 `json:"completed"`
}

type opencodePath struct {
	CWD  string `json:"cwd"`
	Root string `json:"root"`
}

// Scan reads all configured opencode database files and extracts new usage records.
func (c *OpenCodeCollector) Scan() error {
	for _, dbPath := range c.paths {
		if err := c.processDB(dbPath); err != nil {
			log.Printf("opencode: error processing %s: %v", dbPath, err)
		}
	}
	return nil
}

func (c *OpenCodeCollector) processDB(dbPath string) error {
	// Get watermark: last processed time_created (stored as offset in file_state)
	_, lastWatermark, err := c.db.GetFileState(dbPath)
	if err != nil {
		return err
	}

	// Open opencode db read-only
	srcDB, err := sql.Open("sqlite", dbPath+"?mode=ro&_pragma=journal_mode(wal)&_pragma=busy_timeout(3000)")
	if err != nil {
		return fmt.Errorf("open opencode db: %w", err)
	}
	defer srcDB.Close()

	// Query assistant messages newer than watermark
	rows, err := srcDB.Query(`
		SELECT m.data, m.session_id, m.time_created, s.directory
		FROM message m
		JOIN session s ON m.session_id = s.id
		WHERE m.time_created > ?
		ORDER BY m.time_created`,
		lastWatermark,
	)
	if err != nil {
		return fmt.Errorf("query opencode messages: %w", err)
	}
	defer rows.Close()

	var records []*storage.UsageRecord
	sessions := map[string]*storage.SessionRecord{}
	var maxWatermark int64

	for rows.Next() {
		var dataJSON string
		var sessionID string
		var timeCreated int64
		var directory string
		if err := rows.Scan(&dataJSON, &sessionID, &timeCreated, &directory); err != nil {
			continue
		}

		var msg opencodeMessageData
		if err := json.Unmarshal([]byte(dataJSON), &msg); err != nil {
			continue
		}
		if msg.Role != "assistant" || msg.ModelID == "" {
			continue
		}
		// Skip zero-token entries (failed API calls)
		if msg.Tokens.Input == 0 && msg.Tokens.Output == 0 {
			continue
		}

		ts := time.UnixMilli(msg.Time.Created)
		if msg.Time.Created == 0 {
			ts = time.UnixMilli(timeCreated)
		}

		rec := &storage.UsageRecord{
			Source:                   "opencode",
			SessionID:                sessionID,
			Model:                    msg.ModelID,
			Timestamp:                ts,
			Project:                  directory,
			InputTokens:              msg.Tokens.Input,
			OutputTokens:             msg.Tokens.Output,
			CacheReadInputTokens:     msg.Tokens.Cache.Read,
			CacheCreationInputTokens: msg.Tokens.Cache.Write,
			ReasoningOutputTokens:    msg.Tokens.Reasoning,
		}
		records = append(records, rec)

		if timeCreated > maxWatermark {
			maxWatermark = timeCreated
		}

		// Track session metadata
		if _, ok := sessions[sessionID]; !ok {
			sessions[sessionID] = &storage.SessionRecord{
				Source:    "opencode",
				SessionID: sessionID,
				CWD:       directory,
				Project:   directory,
				StartTime: ts,
			}
		}
	}

	// Count user prompts per session
	if len(sessions) > 0 {
		promptRows, err := srcDB.Query(`
			SELECT session_id, COUNT(*) FROM message
			WHERE data LIKE '%"role":"user"%'
			GROUP BY session_id`)
		if err == nil {
			defer promptRows.Close()
			for promptRows.Next() {
				var sid string
				var count int
				if promptRows.Scan(&sid, &count) == nil {
					if s, ok := sessions[sid]; ok {
						s.Prompts = count
					}
				}
			}
		}
	}

	if len(records) > 0 {
		if err := c.db.InsertUsageBatch(records); err != nil {
			return fmt.Errorf("insert opencode usage: %w", err)
		}
	}

	for _, sess := range sessions {
		if err := c.db.UpsertSession(sess); err != nil {
			return fmt.Errorf("upsert opencode session: %w", err)
		}
	}

	if maxWatermark > lastWatermark {
		return c.db.SetFileState(dbPath, maxWatermark, maxWatermark)
	}
	return nil
}
