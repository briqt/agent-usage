package collector

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/briqt/agent-usage/internal/storage"
)

// KiroCollector scans Kiro CLI session files and extracts usage records.
type KiroCollector struct {
	db    *storage.DB
	paths []string
}

// NewKiroCollector creates a KiroCollector that scans the given base paths.
func NewKiroCollector(db *storage.DB, paths []string) *KiroCollector {
	return &KiroCollector{db: db, paths: paths}
}

// Scan walks all configured paths and processes Kiro session files.
// Directory structure: <basePath>/<session-id>.json + <session-id>.jsonl
func (c *KiroCollector) Scan() error {
	for _, basePath := range c.paths {
		entries, err := os.ReadDir(basePath)
		if err != nil {
			log.Printf("kiro: cannot read %s: %v", basePath, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".lock") {
				continue
			}
			jsonPath := filepath.Join(basePath, entry.Name())
			if err := c.processSession(jsonPath); err != nil {
				log.Printf("kiro: error processing %s: %v", jsonPath, err)
			}
		}
	}
	return nil
}
