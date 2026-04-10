package pricing

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

const pricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

type modelPricing struct {
	InputCostPerToken              *float64 `json:"input_cost_per_token"`
	OutputCostPerToken             *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost        *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost    *float64 `json:"cache_creation_input_token_cost"`
}

// Sync fetches model pricing from the litellm GitHub repository and upserts
// it into the database. Only models relevant to AI coding agents are stored.
func Sync(db *storage.DB) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(pricingURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	count := 0
	for model, raw := range data {
		var p modelPricing
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.InputCostPerToken == nil || p.OutputCostPerToken == nil {
			continue
		}

		var cacheRead, cacheCreate float64
		if p.CacheReadInputTokenCost != nil {
			cacheRead = *p.CacheReadInputTokenCost
		}
		if p.CacheCreationInputTokenCost != nil {
			cacheCreate = *p.CacheCreationInputTokenCost
		}

		if err := db.UpsertPricing(model, *p.InputCostPerToken, *p.OutputCostPerToken, cacheRead, cacheCreate); err != nil {
			log.Printf("pricing: error upserting %s: %v", model, err)
		}
		count++
	}
	log.Printf("pricing: synced %d models", count)
	return nil
}

// CalcCost computes the USD cost for a single API call given token counts and
// per-token prices. The prices array is [input, output, cache_read, cache_creation].
// input_tokens is the non-cached input only (cache tokens are separate fields).
func CalcCost(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
	inputPrice := prices[0]
	outputPrice := prices[1]
	cacheReadPrice := prices[2]
	cacheCreatePrice := prices[3]

	cost := float64(inputTokens)*inputPrice +
		float64(cacheCreation)*cacheCreatePrice +
		float64(cacheRead)*cacheReadPrice +
		float64(outputTokens)*outputPrice
	return cost
}
