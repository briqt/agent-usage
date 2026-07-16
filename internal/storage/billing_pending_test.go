package storage

import (
	"testing"
	"time"
)

func TestRecalcPendingCostsOnlyTouchesNewRows(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "test", SessionID: "old", Model: "model",
		InputTokens: 10, Timestamp: base,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecalcCosts(map[string]ModelPricing{
		"model": {Input: 1, Source: "first"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecalcPendingCosts(map[string]ModelPricing{
		"model": {Input: 2, Source: "second"},
	}); err != nil {
		t.Fatal(err)
	}
	var oldCost float64
	if err := db.db.QueryRow("SELECT cost_usd FROM usage_records WHERE session_id='old'").Scan(&oldCost); err != nil {
		t.Fatal(err)
	}
	if oldCost != 10 {
		t.Fatalf("already-priced row changed to %f", oldCost)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "test", SessionID: "new", Model: "model",
		InputTokens: 5, Timestamp: base.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecalcPendingCosts(map[string]ModelPricing{
		"model": {Input: 2, Source: "second"},
	}); err != nil {
		t.Fatal(err)
	}
	var newCost float64
	if err := db.db.QueryRow("SELECT cost_usd FROM usage_records WHERE session_id='new'").Scan(&newCost); err != nil {
		t.Fatal(err)
	}
	if newCost != 10 {
		t.Fatalf("new row cost = %f, want 10", newCost)
	}
	if err := db.RecalcCosts(map[string]ModelPricing{
		"model": {Input: 2, Source: "second"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow("SELECT cost_usd FROM usage_records WHERE session_id='old'").Scan(&oldCost); err != nil {
		t.Fatal(err)
	}
	if oldCost != 20 {
		t.Fatalf("full rate-card refresh left old cost at %f", oldCost)
	}
}
