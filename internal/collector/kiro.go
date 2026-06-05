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

// Scan walks configured paths and processes kiro data.sqlite3 databases.
func (c *KiroCollector) Scan() error {
	for _, basePath := range c.paths {
		info, err := os.Stat(basePath)
		if err != nil {
			log.Printf("kiro: cannot read %s: %v", basePath, err)
			continue
		}

		dbPath := basePath
		if info.IsDir() {
			dbPath = filepath.Join(basePath, "data.sqlite3")
		}
		if !isKiroSQLitePath(dbPath) {
			log.Printf("kiro: skipping non-SQLite path %s", basePath)
			continue
		}
		if _, err := os.Stat(dbPath); err != nil {
			log.Printf("kiro: cannot read %s: %v", dbPath, err)
			continue
		}
		if err := c.processSQLite(dbPath); err != nil {
			log.Printf("kiro: error processing %s: %v", dbPath, err)
		}
	}
	return nil
}

func isKiroSQLitePath(path string) bool {
	base := filepath.Base(path)
	return base == "data.sqlite3" || strings.HasSuffix(base, ".sqlite") || strings.HasSuffix(base, ".sqlite3") || strings.HasSuffix(base, ".db")
}
