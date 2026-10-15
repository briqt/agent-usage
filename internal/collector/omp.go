package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/briqt/agent-usage/internal/storage"
)

// OMPCollector scans Oh My Pi JSONL transcripts, including nested subagents.
type OMPCollector struct {
	db    *storage.DB
	paths []string
}

// NewOMPCollector creates an Oh My Pi collector.
func NewOMPCollector(db *storage.DB, paths []string) *OMPCollector {
	return &OMPCollector{db: db, paths: paths}
}

type ompEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Version   int             `json:"version"`
	CWD       string          `json:"cwd"`
	Model     string          `json:"model"`
	Message   json.RawMessage `json:"message"`
}

type ompMessage struct {
	Role     string    `json:"role"`
	Model    string    `json:"model"`
	Provider string    `json:"provider"`
	Usage    *ompUsage `json:"usage"`
}

type ompUsage struct {
	Input           int64   `json:"input"`
	Output          int64   `json:"output"`
	CacheRead       int64   `json:"cacheRead"`
	CacheWrite      int64   `json:"cacheWrite"`
	ReasoningTokens int64   `json:"reasoningTokens"`
	Cost            ompCost `json:"cost"`
}

type ompCost struct {
	Total float64 `json:"total"`
}

// Scan walks every configured OMP sessions directory recursively.
func (c *OMPCollector) Scan() error {
	for _, basePath := range c.paths {
		if _, err := os.Stat(basePath); err != nil {
			log.Printf("omp: cannot read %s: %v", basePath, err)
			continue
		}
		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if err := c.processFile(path); err != nil {
				log.Printf("omp: error processing %s: %v", path, err)
			}
			return nil
		})
	}
	return nil
}
