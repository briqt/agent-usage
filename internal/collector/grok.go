package collector

import (
	"log"
	"os"
	"path/filepath"

	"github.com/briqt/agent-usage/internal/storage"
)

// GrokCollector scans Grok Build completed-turn usage ledgers.
type GrokCollector struct {
	db    *storage.DB
	paths []string
}

// NewGrokCollector creates a Grok Build collector.
func NewGrokCollector(db *storage.DB, paths []string) *GrokCollector {
	return &GrokCollector{db: db, paths: paths}
}

// Scan finds the authoritative updates.jsonl file for each Grok session.
func (c *GrokCollector) Scan() error {
	for _, basePath := range c.paths {
		if _, err := os.Stat(basePath); err != nil {
			log.Printf("grok: cannot read %s: %v", basePath, err)
			continue
		}
		_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() || info.Name() != "updates.jsonl" {
				return nil
			}
			if err := c.processFile(path); err != nil {
				log.Printf("grok: error processing %s: %v", path, err)
			}
			return nil
		})
	}
	return nil
}
