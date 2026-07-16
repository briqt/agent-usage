package pricing

import "github.com/briqt/agent-usage/internal/storage"

// officialOverrides is intentionally small. LiteLLM remains the broad model
// catalog; official overrides fill product-specific fields it cannot express.
var officialOverrides = map[string]storage.ModelPricing{
	"claude-opus-4-8": {
		Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6,
		CacheCreation5m: 6.25e-6, CacheCreation1h: 10e-6,
		FastMultiplier: 2, Source: "anthropic_official",
	},
	"claude-opus-4-7": {
		Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6,
		CacheCreation5m: 6.25e-6, CacheCreation1h: 10e-6,
		FastMultiplier: 6, Source: "anthropic_official",
	},
	"claude-opus-4-6": {
		Input: 5e-6, Output: 25e-6, CacheRead: 0.5e-6,
		CacheCreation5m: 6.25e-6, CacheCreation1h: 10e-6,
		FastMultiplier: 1, Source: "anthropic_official",
	},
	"claude-sonnet-4-6": {
		Input: 3e-6, Output: 15e-6, CacheRead: 0.3e-6,
		CacheCreation5m: 3.75e-6, CacheCreation1h: 6e-6,
		FastMultiplier: 1, Source: "anthropic_official",
	},
	"claude-haiku-4-5": {
		Input: 1e-6, Output: 5e-6, CacheRead: 0.1e-6,
		CacheCreation5m: 1.25e-6, CacheCreation1h: 2e-6,
		FastMultiplier: 1, Source: "anthropic_official",
	},
}
