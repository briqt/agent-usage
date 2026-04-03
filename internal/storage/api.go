package storage

import "time"

// DashboardStats holds aggregate statistics for the dashboard summary cards.
type DashboardStats struct {
	TotalCost     float64 `json:"total_cost"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalSessions int     `json:"total_sessions"`
	TotalPrompts  int     `json:"total_prompts"`
}

// CostByModel represents total cost for a single model.
type CostByModel struct {
	Model string  `json:"model"`
	Cost  float64 `json:"cost"`
}

// TimeSeriesPoint represents a single data point in a daily cost time series.
type TimeSeriesPoint struct {
	Date   string  `json:"date"`
	Value  float64 `json:"value"`
	Model  string  `json:"model,omitempty"`
}

// TokenTimeSeriesPoint represents daily token usage broken down by category.
type TokenTimeSeriesPoint struct {
	Date         string `json:"date"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheCreate  int64  `json:"cache_create"`
}

// SessionInfo represents a session with aggregated cost and token totals.
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

// GetDashboardStats returns aggregate cost, token, session, and prompt counts
// for usage records within the given time range.
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

// GetCostByModel returns total cost grouped by model within the given time range.
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

// granularityExpr returns a SQL expression that truncates a timestamp column
// to the bucket boundary for the given granularity code.
// Timestamps are stored as Go time strings like "2026-04-03 09:51:45.996 +0000 UTC",
// so we use SUBSTR-based extraction since SQLite's STRFTIME cannot parse this format.
// Supported codes: 1m, 30m, 1h, 6h, 12h, 1d, 1w, 1M.
func granularityExpr(g string) string {
	switch g {
	case "1m":
		// "2026-04-03 09:51" — truncate to minute
		return `SUBSTR(timestamp,1,16)`
	case "30m":
		// floor minute to 00 or 30
		return `SUBSTR(timestamp,1,14) || PRINTF('%02d', (CAST(SUBSTR(timestamp,15,2) AS INTEGER)/30)*30)`
	case "1h":
		// "2026-04-03 09" — truncate to hour
		return `SUBSTR(timestamp,1,13)`
	case "6h":
		// floor hour to 0,6,12,18
		return `SUBSTR(timestamp,1,11) || PRINTF('%02d', (CAST(SUBSTR(timestamp,12,2) AS INTEGER)/6)*6)`
	case "12h":
		// floor hour to 0 or 12
		return `SUBSTR(timestamp,1,11) || PRINTF('%02d', (CAST(SUBSTR(timestamp,12,2) AS INTEGER)/12)*12)`
	case "1w":
		// Use date() with weekday modifier — works because SUBSTR(ts,1,10) is a valid date
		return `DATE(SUBSTR(timestamp,1,10), 'weekday 0', '-6 days')`
	case "1M":
		return `SUBSTR(timestamp,1,7)`
	default: // "1d" or unknown
		return `SUBSTR(timestamp,1,10)`
	}
}

// GetCostOverTime returns cost per model grouped by the given granularity within the time range.
func (d *DB) GetCostOverTime(from, to time.Time, granularity string) ([]TimeSeriesPoint, error) {
	expr := granularityExpr(granularity)
	rows, err := d.db.Query(`SELECT `+expr+` as d, model, SUM(cost_usd) as cost
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

// GetTokensOverTime returns token usage breakdown grouped by the given granularity within the time range.
func (d *DB) GetTokensOverTime(from, to time.Time, granularity string) ([]TokenTimeSeriesPoint, error) {
	expr := granularityExpr(granularity)
	rows, err := d.db.Query(`SELECT `+expr+` as d,
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

// GetSessions returns sessions with aggregated cost and token totals within the given time range.
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
