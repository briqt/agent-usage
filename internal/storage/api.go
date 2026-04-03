package storage

import "time"

type DashboardStats struct {
	TotalCost     float64 `json:"total_cost"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalSessions int     `json:"total_sessions"`
	TotalPrompts  int     `json:"total_prompts"`
}

type CostByModel struct {
	Model string  `json:"model"`
	Cost  float64 `json:"cost"`
}

type TimeSeriesPoint struct {
	Date   string  `json:"date"`
	Value  float64 `json:"value"`
	Model  string  `json:"model,omitempty"`
}

type TokenTimeSeriesPoint struct {
	Date         string `json:"date"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheCreate  int64  `json:"cache_create"`
}

type SessionInfo struct {
	SessionID string  `json:"session_id"`
	Source    string  `json:"source"`
	Project   string  `json:"project"`
	CWD       string  `json:"cwd"`
	GitBranch string  `json:"git_branch"`
	StartTime string  `json:"start_time"`
	Prompts   int     `json:"prompts"`
	TotalCost float64 `json:"total_cost"`
	Tokens    int64   `json:"tokens"`
}

func (d *DB) GetDashboardStats(from, to time.Time) (*DashboardStats, error) {
	s := &DashboardStats{}
	err := d.db.QueryRow(`SELECT COALESCE(SUM(cost_usd),0), COALESCE(SUM(input_tokens+output_tokens),0)
		FROM usage_records WHERE timestamp BETWEEN ? AND ?`, from, to).Scan(&s.TotalCost, &s.TotalTokens)
	if err != nil {
		return nil, err
	}
	d.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM usage_records WHERE timestamp BETWEEN ? AND ?`, from, to).Scan(&s.TotalSessions)
	d.db.QueryRow(`SELECT COALESCE(SUM(prompts),0) FROM sessions WHERE session_id IN
		(SELECT DISTINCT session_id FROM usage_records WHERE timestamp BETWEEN ? AND ?)`, from, to).Scan(&s.TotalPrompts)
	return s, nil
}

func (d *DB) GetCostByModel(from, to time.Time) ([]CostByModel, error) {
	rows, err := d.db.Query(`SELECT model, SUM(cost_usd) as cost FROM usage_records
		WHERE timestamp BETWEEN ? AND ? GROUP BY model ORDER BY cost DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CostByModel
	for rows.Next() {
		var r CostByModel
		if err := rows.Scan(&r.Model, &r.Cost); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *DB) GetCostOverTime(from, to time.Time) ([]TimeSeriesPoint, error) {
	rows, err := d.db.Query(`SELECT SUBSTR(timestamp,1,10) as d, model, SUM(cost_usd) as cost
		FROM usage_records WHERE timestamp BETWEEN ? AND ?
		GROUP BY d, model ORDER BY d`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Date, &p.Model, &p.Value); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *DB) GetTokensOverTime(from, to time.Time) ([]TokenTimeSeriesPoint, error) {
	rows, err := d.db.Query(`SELECT SUBSTR(timestamp,1,10) as d,
		SUM(input_tokens) as inp, SUM(output_tokens) as outp,
		SUM(cache_read_input_tokens) as cr, SUM(cache_creation_input_tokens) as cc
		FROM usage_records WHERE timestamp BETWEEN ? AND ?
		GROUP BY d ORDER BY d`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TokenTimeSeriesPoint
	for rows.Next() {
		var p TokenTimeSeriesPoint
		if err := rows.Scan(&p.Date, &p.InputTokens, &p.OutputTokens, &p.CacheRead, &p.CacheCreate); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *DB) GetSessions(from, to time.Time) ([]SessionInfo, error) {
	rows, err := d.db.Query(`SELECT s.session_id, s.source, s.project, s.cwd, s.git_branch,
		COALESCE(s.start_time,''), s.prompts,
		COALESCE(u.cost,0), COALESCE(u.tokens,0)
		FROM sessions s
		LEFT JOIN (SELECT session_id, SUM(cost_usd) as cost, SUM(input_tokens+output_tokens) as tokens
			FROM usage_records WHERE timestamp BETWEEN ? AND ? GROUP BY session_id) u
		ON s.session_id = u.session_id
		WHERE u.session_id IS NOT NULL
		ORDER BY s.start_time DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.SessionID, &s.Source, &s.Project, &s.CWD, &s.GitBranch, &s.StartTime, &s.Prompts, &s.TotalCost, &s.Tokens); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
