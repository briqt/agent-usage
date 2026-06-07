package collector

import (
	"log"
	"os"
	"path/filepath"

	"github.com/briqt/agent-usage/internal/storage"
)

// HermesCollector scans Hermes Agent state.db files and extracts usage records.
type HermesCollector struct {
	db    *storage.DB
	paths []string
}

// NewHermesCollector creates a HermesCollector that scans the given base paths.
func NewHermesCollector(db *storage.DB, paths []string) *HermesCollector {
	return &HermesCollector{db: db, paths: paths}
}

// Scan discovers all Hermes state.db files and processes them.
// It looks for:
//   - <basePath>/state.db (global instance)
//   - <basePath>/profiles/*/state.db (per-profile instances)
func (c *HermesCollector) Scan() error {
	if err := c.db.DeleteBySource("hermes"); err != nil {
		return err
	}

	for _, basePath := range c.paths {
		// Global state.db
		globalDB := filepath.Join(basePath, "state.db")
		if info, err := os.Stat(globalDB); err == nil && !info.IsDir() {
			if err := c.processDB(globalDB, "hermes"); err != nil {
				log.Printf("hermes: error processing %s: %v", globalDB, err)
			}
		}

		// Profile state.db files
		profilesDir := filepath.Join(basePath, "profiles")
		entries, err := os.ReadDir(profilesDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			profileDB := filepath.Join(profilesDir, entry.Name(), "state.db")
			if info, err := os.Stat(profileDB); err == nil && !info.IsDir() {
				if err := c.processDB(profileDB, entry.Name()); err != nil {
					log.Printf("hermes: error processing %s: %v", profileDB, err)
				}
			}
		}
	}
	return nil
}
