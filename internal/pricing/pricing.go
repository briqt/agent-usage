package pricing

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"devobs/internal/storage"
)

const pricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

type modelPricing struct {
	InputCostPerToken              *float64 `json:"input_cost_per_token"`
	OutputCostPerToken             *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost        *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost    *float64 `json:"cache_creation_input_token_cost"`
}

func Sync(db *storage.DB) error {
	resp, err := http.Get(pricingURL)
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
		// Only keep models we care about (claude, gpt, o1, o3, o4)
		lower := strings.ToLower(model)
		if !strings.Contains(lower, "claude") &&
			!strings.Contains(lower, "gpt") &&
			!strings.Contains(lower, "o1-") &&
			!strings.Contains(lower, "o3-") &&
			!strings.Contains(lower, "o4-") {
			continue
		}

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

func CalcCost(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
	inputPrice := prices[0]
	outputPrice := prices[1]
	cacheReadPrice := prices[2]
	cacheCreatePrice := prices[3]

	regularInput := inputTokens - cacheRead - cacheCreation
	if regularInput < 0 {
		regularInput = 0
	}

	cost := float64(regularInput)*inputPrice +
		float64(cacheCreation)*cacheCreatePrice +
		float64(cacheRead)*cacheReadPrice +
		float64(outputTokens)*outputPrice
	return cost
}
