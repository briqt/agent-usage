package collector

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/briqt/agent-usage/internal/storage"
)

// KiroCollector scans the kiro SQLite database and extracts usage records.
type KiroCollector struct {
	db    *storage.DB
	paths []string
}

// NewKiroCollector creates a KiroCollector that scans the given database paths.
func NewKiroCollector(db *storage.DB, paths []string) *KiroCollector {
	return &KiroCollector{db: db, paths: paths}
}

// Scan walks configured paths and processes kiro sessions from both SQLite
// databases and JSON/JSONL session files.
func (c *KiroCollector) Scan() error {
	for _, basePath := range c.paths {
		info, err := os.Stat(basePath)
		if err != nil {
			log.Printf("kiro: cannot read %s: %v", basePath, err)
			continue
		}

		if !info.IsDir() {
			// Direct file path — must be SQLite.
			if isKiroSQLitePath(basePath) {
				if err := c.processSQLite(basePath); err != nil {
					log.Printf("kiro: error processing %s: %v", basePath, err)
				}
			}
			continue
		}

		// Directory: check for data.sqlite3 first.
		dbPath := filepath.Join(basePath, "data.sqlite3")
		if _, err := os.Stat(dbPath); err == nil {
			if err := c.processSQLite(dbPath); err != nil {
				log.Printf("kiro: error processing %s: %v", dbPath, err)
			}
			continue
		}

		// Directory without data.sqlite3: scan as JSON session directory.
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

func isKiroSQLitePath(path string) bool {
	base := filepath.Base(path)
	return base == "data.sqlite3" || strings.HasSuffix(base, ".sqlite") || strings.HasSuffix(base, ".sqlite3") || strings.HasSuffix(base, ".db")
}
