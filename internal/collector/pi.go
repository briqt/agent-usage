package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/briqt/agent-usage/internal/storage"
)

// PiCollector scans Pi coding agent session JSONL files and extracts usage records.
type PiCollector struct {
	db    *storage.DB
	paths []string
}

// NewPiCollector creates a PiCollector that scans the given base paths.
func NewPiCollector(db *storage.DB, paths []string) *PiCollector {
	return &PiCollector{db: db, paths: paths}
}

type piEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Version   int             `json:"version"`
	CWD       string          `json:"cwd"`
	ModelID   string          `json:"modelId"`
	Provider  string          `json:"provider"`
	Message   json.RawMessage `json:"message"`
}

type piMessage struct {
	Role     string          `json:"role"`
	Content  json.RawMessage `json:"content"`
	Model    string          `json:"model"`
	Provider string          `json:"provider"`
	Usage    *piUsage        `json:"usage"`
}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// Scan walks all configured paths and processes new JSONL data from Pi sessions.
// Directory structure: <basePath>/<workspace-slug>/<file>.jsonl
func (c *PiCollector) Scan() error {
	for _, basePath := range c.paths {
		workspaces, err := os.ReadDir(basePath)
		if err != nil {
			log.Printf("pi: cannot read %s: %v", basePath, err)
			continue
		}
		for _, ws := range workspaces {
			if !ws.IsDir() {
				continue
			}
			slug := ws.Name()
			project := deriveProjectFromSlug(slug)
			wsDir := filepath.Join(basePath, slug)
			filepath.Walk(wsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
					return nil
				}
				if err := c.processFile(path, project); err != nil {
					log.Printf("pi: error processing %s: %v", path, err)
				}
				return nil
			})
		}
	}
	return nil
}

// deriveProjectFromSlug extracts a project name from a Pi workspace slug.
// Slugs encode the workspace path with dashes replacing slashes and
// leading/trailing double-dashes, e.g.:
//
//	"--home-wangl-codes-agent-usage--" → "agent-usage"
//
// Since dashes are ambiguous (path separator vs. literal dash in names),
// this uses a heuristic: take the last dash-separated token. If the CWD is
// available from the session entry, processFile will override with the actual
// last path component.
func deriveProjectFromSlug(slug string) string {
	// Strip leading/trailing "--"
	s := strings.TrimPrefix(slug, "--")
	s = strings.TrimSuffix(s, "--")
	if s == "" {
		return slug
	}
	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return slug
	}
	// Return the last non-empty token
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return slug
}
