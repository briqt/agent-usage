package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/briqt/agent-usage/internal/storage"
)

// OpenClawCollector scans OpenClaw session JSONL files and extracts usage records.
type OpenClawCollector struct {
	db    *storage.DB
	paths []string
}

// NewOpenClawCollector creates an OpenClawCollector that scans the given base paths.
func NewOpenClawCollector(db *storage.DB, paths []string) *OpenClawCollector {
	return &OpenClawCollector{db: db, paths: paths}
}

type openclawEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  string          `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Version   int             `json:"version"`
	CWD       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`
}

type openclawMessage struct {
	Role     string          `json:"role"`
	Content  json.RawMessage `json:"content"`
	Model    string          `json:"model"`
	Provider string          `json:"provider"`
	Usage    *openclawUsage  `json:"usage"`
}

type openclawUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// Scan walks all configured paths and processes new JSONL data from OpenClaw sessions.
// Directory structure: <basePath>/<agentId>/sessions/<sessionId>.jsonl
func (c *OpenClawCollector) Scan() error {
	for _, basePath := range c.paths {
		agents, err := os.ReadDir(basePath)
		if err != nil {
			log.Printf("openclaw: cannot read %s: %v", basePath, err)
			continue
		}
		for _, agent := range agents {
			if !agent.IsDir() {
				continue
			}
			agentID := agent.Name()
			sessionsDir := filepath.Join(basePath, agentID, "sessions")
			filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
					return nil
				}
				if err := c.processFile(path, agentID); err != nil {
					log.Printf("openclaw: error processing %s: %v", path, err)
				}
				return nil
			})
		}
	}
	return nil
}
