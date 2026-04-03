package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/briqt/agent-usage/internal/storage"
)

// ClaudeCollector scans Claude Code session JSONL files and extracts usage records.
type ClaudeCollector struct {
	db    *storage.DB
	paths []string
}

// NewClaudeCollector creates a ClaudeCollector that scans the given base paths.
func NewClaudeCollector(db *storage.DB, paths []string) *ClaudeCollector {
	return &ClaudeCollector{db: db, paths: paths}
}

type claudeEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Version   string          `json:"version"`
	CWD       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Message   json.RawMessage `json:"message"`
}

type claudeMessage struct {
	Role  string      `json:"role"`
	Model string      `json:"model"`
	Usage *claudeUsage `json:"usage"`
}

type claudeUsage struct {
	InputTokens              *int64 `json:"input_tokens"`
	OutputTokens             *int64 `json:"output_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
}

// Scan walks all configured paths and processes new JSONL data from Claude Code sessions.
func (c *ClaudeCollector) Scan() error {
	for _, basePath := range c.paths {
		projects, err := os.ReadDir(basePath)
		if err != nil {
			log.Printf("claude: cannot read %s: %v", basePath, err)
			continue
		}
		for _, proj := range projects {
			if !proj.IsDir() {
				continue
			}
			projName := proj.Name()
			projPath := filepath.Join(basePath, projName)
			filepath.Walk(projPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
					return nil
				}
				if err := c.processFile(path, projName); err != nil {
					log.Printf("claude: error processing %s: %v", path, err)
				}
				return nil
			})
		}
	}
	return nil
}
