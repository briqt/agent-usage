package storage

import "strings"

type CostCalcFunc func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64

func (d *DB) RecalcCosts(allPrices map[string][4]float64, calcFn CostCalcFunc) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, model, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens FROM usage_records WHERE cost_usd = 0`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		id                       int64
		model                    string
		input, output, cc, cr    int64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.model, &r.input, &r.output, &r.cc, &r.cr); err != nil {
			return err
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(recs) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE usage_records SET cost_usd=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	updated := 0
	for _, r := range recs {
		prices, ok := matchPricing(r.model, allPrices)
		if !ok {
			continue
		}
		cost := calcFn(r.input, r.output, r.cc, r.cr, prices)
		if cost > 0 {
			if _, err := stmt.Exec(cost, r.id); err != nil {
				return err
			}
			updated++
		}
	}

	if updated > 0 {
		return tx.Commit()
	}
	return nil
}

func matchPricing(model string, allPrices map[string][4]float64) ([4]float64, bool) {
	// Direct match
	if p, ok := allPrices[model]; ok {
		return p, true
	}
	// Try with provider prefix
	for _, prefix := range []string{"anthropic/", "openai/", ""} {
		if p, ok := allPrices[prefix+model]; ok {
			return p, true
		}
	}

	// Normalize: replace / with . and version dots with dashes for matching
	// e.g. "anthropic/claude-sonnet-4.6" -> "anthropic.claude-sonnet-4-6"
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "/", ".")
		return s
	}

	modelNorm := norm(model)
	// Also try normalizing version numbers: 4.6 -> 4-6
	modelNormDash := strings.NewReplacer("4.6", "4-6", "4.5", "4-5", "3.5", "3-5", "5.4", "5-4").Replace(modelNorm)

	var bestKey string
	var bestScore int
	for k := range allPrices {
		kNorm := norm(k)
		for _, mn := range []string{modelNorm, modelNormDash} {
			if strings.Contains(kNorm, mn) || strings.Contains(mn, kNorm) {
				score := len(k)
				if kNorm == mn {
					score += 10000 // exact match bonus
				}
				if score > bestScore {
					bestKey = k
					bestScore = score
				}
			}
		}
	}
	if bestKey != "" {
		p := allPrices[bestKey]
		return p, true
	}
	return [4]float64{}, false
}
