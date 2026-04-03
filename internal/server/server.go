package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"devobs/internal/storage"
)

//go:embed static
var staticFS embed.FS

type Server struct {
	db   *storage.DB
	port int
}

func New(db *storage.DB, port int) *Server {
	return &Server{db: db, port: port}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("server: listening on %s", addr)
	return http.ListenAndServe(addr, mux)
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

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	stats, err := s.db.GetDashboardStats(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleCostByModel(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	data, err := s.db.GetCostByModel(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleCostOverTime(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	data, err := s.db.GetCostOverTime(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleTokensOverTime(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	data, err := s.db.GetTokensOverTime(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseTimeRange(r)
	data, err := s.db.GetSessions(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}
