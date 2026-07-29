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

// dbFingerprint identifies a version of a SQLite database file cheaply, without
// opening it. The -wal file is included because in WAL mode writes land in the
// log and leave the main file's size and mtime unchanged.
type dbFingerprint struct {
	size         int64
	mtimeNano    int64
	walSize      int64
	walMTimeNano int64
}

func fingerprintDB(dbPath string) (dbFingerprint, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return dbFingerprint{}, err
	}
	fp := dbFingerprint{size: info.Size(), mtimeNano: info.ModTime().UnixNano()}
	// A missing -wal is normal (journal_mode may not be WAL, or it was
	// checkpointed away); leave those fields zero.
	if walInfo, err := os.Stat(dbPath + "-wal"); err == nil {
		fp.walSize = walInfo.Size()
		fp.walMTimeNano = walInfo.ModTime().UnixNano()
	}
	return fp, nil
}

// Scan discovers all Hermes state.db files and processes them.
// It looks for:
//   - <basePath>/state.db (global instance)
//   - <basePath>/profiles/*/state.db (per-profile instances)
//
// Each database is gated on its fingerprint, so an idle Hermes instance costs two
// stat calls per interval instead of a full table scan.
func (c *HermesCollector) Scan() error {
	for _, basePath := range c.paths {
		// Global state.db
		globalDB := filepath.Join(basePath, "state.db")
		if info, err := os.Stat(globalDB); err == nil && !info.IsDir() {
			if err := c.syncDB(globalDB, "hermes"); err != nil {
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
				if err := c.syncDB(profileDB, entry.Name()); err != nil {
					log.Printf("hermes: error processing %s: %v", profileDB, err)
				}
			}
		}
	}
	return nil
}

// syncDB skips databases that have not changed since the last scan, and otherwise
// re-syncs this database's project and advances its prompt watermark.
//
// The watermark only advances on success, so a failed scan is retried in full
// rather than silently skipping rows.
func (c *HermesCollector) syncDB(dbPath, project string) error {
	fp, err := fingerprintDB(dbPath)
	if err != nil {
		return err
	}

	prevSize, prevRowID, prevCtx, err := c.db.GetFileState(dbPath)
	if err != nil {
		return err
	}
	if prevCtx != nil &&
		prevSize == fp.size &&
		prevCtx.DBMTimeNano == fp.mtimeNano &&
		prevCtx.WALSize == fp.walSize &&
		prevCtx.WALMTimeNano == fp.walMTimeNano {
		return nil
	}

	newRowID, err := c.processDB(dbPath, project, prevRowID)
	if err != nil {
		return err
	}

	return c.db.SetFileState(dbPath, fp.size, newRowID, &storage.FileScanContext{
		DBMTimeNano:  fp.mtimeNano,
		WALSize:      fp.walSize,
		WALMTimeNano: fp.walMTimeNano,
	})
}
