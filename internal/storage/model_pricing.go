package storage

import "time"

// ModelPricing stores per-token API prices and pricing provenance.
type ModelPricing struct {
	Input           float64
	Output          float64
	CacheRead       float64
	CacheCreation5m float64
	CacheCreation1h float64
	FastMultiplier  float64
	Source          string
}

// UpsertModelPricing inserts or updates the complete rate card for a model.
func (d *DB) UpsertModelPricing(model string, p ModelPricing) error {
	if p.FastMultiplier == 0 {
		p.FastMultiplier = 1
	}
	if p.Source == "" {
		p.Source = "litellm"
	}
	_, err := d.db.Exec(`INSERT INTO pricing(model,input_cost_per_token,output_cost_per_token,
		cache_read_input_token_cost,cache_creation_input_token_cost,
		cache_creation_1h_input_token_cost,fast_multiplier,source,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(model) DO UPDATE SET
			input_cost_per_token=excluded.input_cost_per_token,
			output_cost_per_token=excluded.output_cost_per_token,
			cache_read_input_token_cost=excluded.cache_read_input_token_cost,
			cache_creation_input_token_cost=excluded.cache_creation_input_token_cost,
			cache_creation_1h_input_token_cost=excluded.cache_creation_1h_input_token_cost,
			fast_multiplier=excluded.fast_multiplier,
			source=excluded.source,
			updated_at=excluded.updated_at`,
		model, p.Input, p.Output, p.CacheRead, p.CacheCreation5m,
		p.CacheCreation1h, p.FastMultiplier, p.Source, time.Now())
	return err
}

// GetAllModelPricing returns complete rate cards keyed by model name.
func (d *DB) GetAllModelPricing() (map[string]ModelPricing, error) {
	rows, err := d.db.Query(`SELECT model,input_cost_per_token,output_cost_per_token,
		cache_read_input_token_cost,cache_creation_input_token_cost,
		cache_creation_1h_input_token_cost,fast_multiplier,source FROM pricing`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]ModelPricing)
	for rows.Next() {
		var model string
		var p ModelPricing
		if err := rows.Scan(&model, &p.Input, &p.Output, &p.CacheRead, &p.CacheCreation5m,
			&p.CacheCreation1h, &p.FastMultiplier, &p.Source); err != nil {
			return nil, err
		}
		result[model] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
