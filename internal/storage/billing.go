package storage

import (
	"sort"
	"strings"
	"time"
)

type billingRecord struct {
	id                                  int64
	source, provider, model, speed, geo string
	nativeCost                          float64
	nativeCostKind                      string
	input, output, cacheCreate, cache5m int64
	cache1h, cacheRead                  int64
	timestamp                           time.Time
}

// RecalcCosts refreshes the single effective cost for every record. A
// source-reported cost wins; token pricing is the fallback. Unknown models
// without a reported cost remain unpriced.
func (d *DB) RecalcCosts(allPrices map[string]ModelPricing) error {
	return d.recalcCosts(allPrices, false)
}

// RecalcPendingCosts prices only newly collected rows. Full historical
// recalculation is reserved for rate-card synchronization.
func (d *DB) RecalcPendingCosts(allPrices map[string]ModelPricing) error {
	return d.recalcCosts(allPrices, true)
}

func (d *DB) recalcCosts(allPrices map[string]ModelPricing, onlyPending bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id,source,provider,model,input_tokens,output_tokens,
		cache_creation_input_tokens,cache_creation_5m_tokens,cache_creation_1h_tokens,
		cache_read_input_tokens,speed,inference_geo,native_cost_usd,native_cost_kind,
		timestamp FROM usage_records`
	if onlyPending {
		query += " WHERE priced_at IS NULL"
	}
	rows, err := d.db.Query(query)
	if err != nil {
		return err
	}
	var records []billingRecord
	for rows.Next() {
		var r billingRecord
		if err := rows.Scan(&r.id, &r.source, &r.provider, &r.model, &r.input, &r.output,
			&r.cacheCreate, &r.cache5m, &r.cache1h, &r.cacheRead, &r.speed, &r.geo,
			&r.nativeCost, &r.nativeCostKind, &r.timestamp); err != nil {
			rows.Close()
			return err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(records) == 0 {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS billing_updates (
		id INTEGER PRIMARY KEY, cost_usd REAL,
		price_source TEXT, priced_at DATETIME
	); DELETE FROM billing_updates;`); err != nil {
		return err
	}

	now := time.Now().UTC()
	matcher := newPricingMatcher(allPrices)
	const batchSize = 400
	for start := 0; start < len(records); start += batchSize {
		end := start + batchSize
		if end > len(records) {
			end = len(records)
		}
		var statement strings.Builder
		statement.WriteString("INSERT INTO billing_updates(id,cost_usd,price_source,priced_at) VALUES ")
		args := make([]any, 0, (end-start)*4)
		for i, r := range records[start:end] {
			if i > 0 {
				statement.WriteByte(',')
			}
			statement.WriteString("(?,?,?,?)")
			cost := 0.0
			priceSource := "unknown"
			if r.nativeCost > 0 {
				cost = r.nativeCost
				priceSource = nativePriceSource(r.nativeCostKind)
			} else {
				if p, ok := matcher.match(r.provider, r.model); ok {
					cost = calculateTokenCost(r, p)
					priceSource = p.Source
					if priceSource == "" {
						priceSource = "litellm"
					}
				}
			}
			args = append(args, r.id, cost, priceSource, now)
		}
		if _, err := tx.Exec(statement.String(), args...); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE usage_records
		SET (cost_usd,price_source,priced_at) =
			(SELECT cost_usd,price_source,priced_at
			 FROM billing_updates WHERE billing_updates.id=usage_records.id)
		WHERE id IN (SELECT id FROM billing_updates)`); err != nil {
		return err
	}
	return tx.Commit()
}

func nativePriceSource(kind string) string {
	switch kind {
	case "actual":
		return "source_reported"
	case "source_estimate":
		return "source_reported_estimate"
	default:
		return "source_reported"
	}
}

func calculateTokenCost(r billingRecord, p ModelPricing) float64 {
	cache5m, cache1h := r.cache5m, r.cache1h
	if cache5m == 0 && cache1h == 0 {
		cache5m = r.cacheCreate
	}
	cache1hPrice := p.CacheCreation1h
	if cache1hPrice == 0 {
		cache1hPrice = p.CacheCreation5m
	}
	cost := float64(r.input)*p.Input +
		float64(r.output)*p.Output +
		float64(r.cacheRead)*p.CacheRead +
		float64(cache5m)*p.CacheCreation5m +
		float64(cache1h)*cache1hPrice
	if strings.EqualFold(r.speed, "fast") && p.FastMultiplier > 0 {
		cost *= p.FastMultiplier
	}
	// Anthropic's US-only inference modifier applies to the full request.
	if p.Source == "anthropic_official" && r.source == "claude" && strings.EqualFold(r.geo, "us") {
		cost *= 1.1
	}
	return cost
}

type pricingMatcher struct {
	exact      map[string]ModelPricing
	normalized map[string]ModelPricing
}

func newPricingMatcher(all map[string]ModelPricing) pricingMatcher {
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	normalized := make(map[string]ModelPricing, len(all))
	for _, key := range keys {
		normalizedKey := normalizeModelPriceKey(key)
		if _, exists := normalized[normalizedKey]; !exists {
			normalized[normalizedKey] = all[key]
		}
	}
	return pricingMatcher{exact: all, normalized: normalized}
}

func normalizeModelPriceKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "models/")
	return strings.NewReplacer(
		"4.8", "4-8", "4.7", "4-7", "4.6", "4-6", "4.5", "4-5",
		"5.6", "5-6", "5.5", "5-5", "5.4", "5-4", "5.3", "5-3", "5.2", "5-2",
	).Replace(value)
}

func (m pricingMatcher) match(provider, model string) (ModelPricing, bool) {
	model = strings.TrimSpace(model)
	provider = strings.Trim(strings.ToLower(strings.TrimSpace(provider)), "/")
	var candidates []string
	if provider != "" {
		candidates = append(candidates, provider+"/"+model)
	}
	candidates = append(candidates, model)
	for _, prefix := range []string{"anthropic/", "openai/", "google/", "gemini/", "deepseek/", "mistral/", "cohere/", "azure_ai/"} {
		candidates = append(candidates, prefix+model)
	}
	for _, candidate := range candidates {
		if p, ok := m.exact[candidate]; ok {
			return p, true
		}
	}
	for _, candidate := range candidates {
		if p, ok := m.normalized[normalizeModelPriceKey(candidate)]; ok {
			return p, true
		}
	}
	return ModelPricing{}, false
}

func matchModelPricing(provider, model string, all map[string]ModelPricing) (ModelPricing, bool) {
	return newPricingMatcher(all).match(provider, model)
}
