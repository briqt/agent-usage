package main

import (
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
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer db.Close()

	// Sync pricing
	log.Println("syncing pricing data...")
	if err := pricing.Sync(db); err != nil {
		log.Printf("pricing sync failed: %v (continuing without pricing)", err)
	}

	// Calculate costs for existing records
	recalcCosts(db)

	// Initial scan
	if cfg.Collectors.Claude.Enabled {
		cc := collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths)
		log.Println("scanning Claude Code sessions...")
		if err := cc.Scan(); err != nil {
			log.Printf("claude scan: %v", err)
		}
		recalcCosts(db)

		// Background scanner
		go func() {
			ticker := time.NewTicker(cfg.Collectors.Claude.ScanInterval)
			for range ticker.C {
				cc.Scan()
				recalcCosts(db)
			}
		}()
	}

	if cfg.Collectors.Codex.Enabled {
		cx := collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths)
		log.Println("scanning Codex sessions...")
		if err := cx.Scan(); err != nil {
			log.Printf("codex scan: %v", err)
		}
		recalcCosts(db)

		go func() {
			ticker := time.NewTicker(cfg.Collectors.Codex.ScanInterval)
			for range ticker.C {
				cx.Scan()
				recalcCosts(db)
			}
		}()
	}

	// Periodic pricing sync
	go func() {
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			pricing.Sync(db)
			recalcCosts(db)
		}
	}()

	// Start web server
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr)
	log.Fatal(srv.Start())
}

func recalcCosts(db *storage.DB) {
	prices, err := db.GetAllPricing()
	if err != nil {
		return
	}
	if err := db.RecalcCosts(prices, pricing.CalcCost); err != nil {
		log.Printf("recalc costs: %v", err)
	}
}
