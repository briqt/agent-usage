package storage

import "testing"

func TestMatchPricingDirectMatch(t *testing.T) {
	prices := map[string][4]float64{
		"claude-opus-4-6": {0.015, 0.075, 0.0015, 0.01875},
	}
	p, ok := matchPricing("claude-opus-4-6", prices)
	if !ok {
		t.Fatal("expected direct match")
	}
	if p[0] != 0.015 {
		t.Errorf("expected input price 0.015, got %f", p[0])
	}
}

func TestMatchPricingProviderPrefix(t *testing.T) {
	prices := map[string][4]float64{
		"deepseek/deepseek-r1": {0.001, 0.002, 0.0005, 0.001},
	}
	p, ok := matchPricing("deepseek-r1", prices)
	if !ok {
		t.Fatal("expected provider prefix match for deepseek/deepseek-r1")
	}
	if p[0] != 0.001 {
		t.Errorf("expected input price 0.001, got %f", p[0])
	}
}

func TestMatchPricingShortestKeyWins(t *testing.T) {
	prices := map[string][4]float64{
		"deepseek/deepseek-r1":                              {0.001, 0.002, 0, 0},
		"fireworks_ai/accounts/fireworks/models/deepseek-r1": {0.009, 0.009, 0, 0},
	}
	// Direct provider prefix match should win
	p, ok := matchPricing("deepseek-r1", prices)
	if !ok {
		t.Fatal("expected match")
	}
	if p[0] != 0.001 {
		t.Errorf("expected original provider price 0.001, got %f (matched reseller)", p[0])
	}
}

func TestMatchPricingFuzzyShortestKey(t *testing.T) {
	// When no direct or provider-prefix match exists, fuzzy match should prefer shortest key
	prices := map[string][4]float64{
		"deepseek-r1":                                        {0.001, 0.002, 0, 0},
		"fireworks_ai/accounts/fireworks/models/deepseek-r1": {0.009, 0.009, 0, 0},
	}
	p, ok := matchPricing("some-deepseek-r1-variant", prices)
	if !ok {
		t.Fatal("expected fuzzy match")
	}
	// Shortest key "deepseek-r1" should win over the long reseller path
	if p[0] != 0.001 {
		t.Errorf("expected shortest key price 0.001, got %f", p[0])
	}
}

func TestMatchPricingVersionNormalization(t *testing.T) {
	prices := map[string][4]float64{
		"claude-sonnet-4-6": {0.003, 0.015, 0.001, 0.004},
	}
	p, ok := matchPricing("claude-sonnet-4.6", prices)
	if !ok {
		t.Fatal("expected version-normalized match (4.6 -> 4-6)")
	}
	if p[0] != 0.003 {
		t.Errorf("expected input price 0.003, got %f", p[0])
	}
}

func TestMatchPricingNoMatch(t *testing.T) {
	prices := map[string][4]float64{
		"claude-opus-4-6": {0.015, 0.075, 0, 0},
	}
	_, ok := matchPricing("totally-unknown-model", prices)
	if ok {
		t.Error("expected no match for unknown model")
	}
}
