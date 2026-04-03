package storage

import (
	"database/sql"
	"time"
)

// File state tracking

func (d *DB) GetFileState(path string) (size, offset int64, err error) {
	err = d.db.QueryRow("SELECT size, last_offset FROM file_state WHERE path=?", path).Scan(&size, &offset)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

func (d *DB) SetFileState(path string, size, offset int64) error {
	_, err := d.db.Exec(`INSERT INTO file_state(path,size,last_offset) VALUES(?,?,?)
		ON CONFLICT(path) DO UPDATE SET size=excluded.size, last_offset=excluded.last_offset`, path, size, offset)
	return err
}

// Sessions

func (d *DB) UpsertSession(s *SessionRecord) error {
	_, err := d.db.Exec(`INSERT INTO sessions(source,session_id,project,cwd,version,git_branch,start_time,prompts)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET
			project=CASE WHEN excluded.project!='' THEN excluded.project ELSE sessions.project END,
			cwd=CASE WHEN excluded.cwd!='' THEN excluded.cwd ELSE sessions.cwd END,
			version=CASE WHEN excluded.version!='' THEN excluded.version ELSE sessions.version END,
			git_branch=CASE WHEN excluded.git_branch!='' THEN excluded.git_branch ELSE sessions.git_branch END,
			start_time=CASE WHEN excluded.start_time < sessions.start_time THEN excluded.start_time ELSE sessions.start_time END,
			prompts=prompts+excluded.prompts`,
		s.Source, s.SessionID, s.Project, s.CWD, s.Version, s.GitBranch, s.StartTime, s.Prompts)
	return err
}

// Usage records

func (d *DB) InsertUsage(r *UsageRecord) error {
	_, err := d.db.Exec(`INSERT INTO usage_records(source,session_id,model,input_tokens,output_tokens,
		cache_creation_input_tokens,cache_read_input_tokens,reasoning_output_tokens,cost_usd,timestamp,project,git_branch)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.Source, r.SessionID, r.Model, r.InputTokens, r.OutputTokens,
		r.CacheCreationInputTokens, r.CacheReadInputTokens, r.ReasoningOutputTokens,
		r.CostUSD, r.Timestamp, r.Project, r.GitBranch)
	return err
}

func (d *DB) InsertUsageBatch(records []*UsageRecord) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO usage_records(source,session_id,model,input_tokens,output_tokens,
		cache_creation_input_tokens,cache_read_input_tokens,reasoning_output_tokens,cost_usd,timestamp,project,git_branch)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		_, err := stmt.Exec(r.Source, r.SessionID, r.Model, r.InputTokens, r.OutputTokens,
			r.CacheCreationInputTokens, r.CacheReadInputTokens, r.ReasoningOutputTokens,
			r.CostUSD, r.Timestamp, r.Project, r.GitBranch)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Pricing

func (d *DB) UpsertPricing(model string, inputCost, outputCost, cacheReadCost, cacheCreationCost float64) error {
	_, err := d.db.Exec(`INSERT INTO pricing(model,input_cost_per_token,output_cost_per_token,
		cache_read_input_token_cost,cache_creation_input_token_cost,updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(model) DO UPDATE SET
			input_cost_per_token=excluded.input_cost_per_token,
			output_cost_per_token=excluded.output_cost_per_token,
			cache_read_input_token_cost=excluded.cache_read_input_token_cost,
			cache_creation_input_token_cost=excluded.cache_creation_input_token_cost,
			updated_at=excluded.updated_at`,
		model, inputCost, outputCost, cacheReadCost, cacheCreationCost, time.Now())
	return err
}

func (d *DB) GetPricing(model string) (inputCost, outputCost, cacheReadCost, cacheCreationCost float64, err error) {
	err = d.db.QueryRow("SELECT input_cost_per_token,output_cost_per_token,cache_read_input_token_cost,cache_creation_input_token_cost FROM pricing WHERE model=?", model).
		Scan(&inputCost, &outputCost, &cacheReadCost, &cacheCreationCost)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, nil
	}
	return
}

func (d *DB) GetAllPricing() (map[string][4]float64, error) {
	rows, err := d.db.Query("SELECT model,input_cost_per_token,output_cost_per_token,cache_read_input_token_cost,cache_creation_input_token_cost FROM pricing")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][4]float64)
	for rows.Next() {
		var model string
		var costs [4]float64
		if err := rows.Scan(&model, &costs[0], &costs[1], &costs[2], &costs[3]); err != nil {
			return nil, err
		}
		m[model] = costs
	}
	return m, nil
}
