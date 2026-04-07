package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/briqt/agent-usage/internal/storage"
)

//go:embed static
var staticFS embed.FS

// Server serves the web dashboard and REST API.
type Server struct {
	db   *storage.DB
	addr string
}

// New creates a Server that will listen on the given address (host:port).
func New(db *storage.DB, addr string) *Server {
	return &Server{db: db, addr: addr}
}

// Start registers HTTP handlers and begins listening. It blocks until the server stops.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session-detail", s.handleSessionDetail)

	log.Printf("server: listening on %s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) parseTimeRange(r *http.Request) (time.Time, time.Time) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	var fromTime, toTime time.Time
	if from != "" {
		fromTime, _ = time.Parse("2006-01-02", from)
	}
	if to != "" {
		toTime, _ = time.Parse("2006-01-02", to)
		toTime = toTime.Add(24*time.Hour - time.Second)
	}
	if fromTime.IsZero() {
		fromTime = time.Now().AddDate(0, -1, 0)
	}
	if toTime.IsZero() {
		toTime = time.Now().Add(24 * time.Hour)
	}
	return fromTime, toTime
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	http.Error(w, "internal server error", 500)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	source := r.URL.Query().Get("source")
	stats, err := s.db.GetDashboardStats(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleCostByModel(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	source := r.URL.Query().Get("source")
	data, err := s.db.GetCostByModel(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleCostOverTime(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	data, err := s.db.GetCostOverTime(from, to, granularity, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleTokensOverTime(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	data, err := s.db.GetTokensOverTime(from, to, granularity, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	source := r.URL.Query().Get("source")
	data, err := s.db.GetSessions(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" {
		http.Error(w, "session_id required", 400)
		return
	}
	data, err := s.db.GetSessionDetail(sid)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}
