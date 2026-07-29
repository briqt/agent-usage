package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/briqt/agent-usage/internal/collector"
	"github.com/briqt/agent-usage/internal/config"
	"github.com/briqt/agent-usage/internal/pricing"
	"github.com/briqt/agent-usage/internal/server"
	"github.com/briqt/agent-usage/internal/storage"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("agent-usage %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(config.ResolveConfigPath(*configPath))
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer db.Close()

	// Versioned migrations own scan-state resets. Resetting on every binary
	// version change duplicates retained history and can lose sessions whose
	// source files have already been removed.
	lastVer, _ := db.GetMeta("version")
	if lastVer != "" && lastVer != version {
		log.Printf("version changed (%s -> %s); applying versioned migrations only", lastVer, version)
	}
	db.SetMeta("version", version)

	// Sync pricing
	log.Println("syncing pricing data...")
	if err := pricing.Sync(db); err != nil {
		log.Printf("pricing sync failed: %v (continuing without pricing)", err)
	}

	// Calculate costs for existing records
	recalcCosts(db)

	// Collector loop
	type collectorEntry struct {
		name string
		c    collector.Collector
		cfg  config.CollectorConfig
	}
	collectors := []collectorEntry{
		{"Claude Code", collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths), cfg.Collectors.Claude},
		{"Codex", collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths), cfg.Collectors.Codex},
		{"OpenClaw", collector.NewOpenClawCollector(db, cfg.Collectors.OpenClaw.Paths), cfg.Collectors.OpenClaw},
		{"OpenCode", collector.NewOpenCodeCollector(db, cfg.Collectors.OpenCode.Paths), cfg.Collectors.OpenCode},
		{"kiro", collector.NewKiroCollector(db, cfg.Collectors.Kiro.Paths), cfg.Collectors.Kiro},
		{"Pi", collector.NewPiCollector(db, cfg.Collectors.Pi.Paths), cfg.Collectors.Pi},
		{"Hermes", collector.NewHermesCollector(db, cfg.Collectors.Hermes.Paths), cfg.Collectors.Hermes},
	}
	for _, ce := range collectors {
		if !ce.cfg.Enabled {
			continue
		}
		log.Printf("scanning %s sessions...", ce.name)
		if err := ce.c.Scan(); err != nil {
			log.Printf("%s scan: %v", ce.name, err)
		}
		recalcPendingCosts(db)

		go func(ce collectorEntry) {
			ticker := time.NewTicker(ce.cfg.ScanInterval)
			for range ticker.C {
				ce.c.Scan()
				recalcPendingCosts(db)
			}
		}(ce)
	}

	// Periodic pricing sync
	go func() {
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			pricing.Sync(db)
			recalcCosts(db)
		}
	}()

	// Periodic WAL checkpoint. The collectors commit on every scan interval, which
	// can starve SQLite's automatic checkpointing indefinitely and let the WAL grow
	// past the database itself — inflating reads, since each one walks the log first.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			if err := db.Checkpoint(); err != nil {
				log.Printf("wal checkpoint: %v", err)
			}
		}
	}()

	// Start web server
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr)
	log.Fatal(srv.Start())
}

func recalcCosts(db *storage.DB) {
	prices, err := db.GetAllModelPricing()
	if err != nil {
		return
	}
	if err := db.RecalcCosts(prices); err != nil {
		log.Printf("recalc costs: %v", err)
	}
}

func recalcPendingCosts(db *storage.DB) {
	prices, err := db.GetAllModelPricing()
	if err != nil {
		return
	}
	if err := db.RecalcPendingCosts(prices); err != nil {
		log.Printf("recalc pending costs: %v", err)
	}
}
