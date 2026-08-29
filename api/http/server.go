package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

// ReadyChecker verifies dependencies for /readyz.
type ReadyChecker interface {
	Ready(ctx context.Context) error
}

// DBReady wraps *sql.DB for readiness probes.
type DBReady struct {
	DB *sql.DB
}

func (d DBReady) Ready(ctx context.Context) error {
	return postgres.Ping(ctx, d.DB)
}

// Server is the Phase 0 HTTP surface.
type Server struct {
	logger *slog.Logger
	ready  ReadyChecker
	mux    *http.ServeMux
}

// New constructs an HTTP server with health endpoints.
func New(logger *slog.Logger, ready ReadyChecker) *Server {
	s := &Server{
		logger: logger,
		ready:  ready,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
}

// Handler returns the root HTTP handler (middleware-ready).
func (s *Server) Handler() http.Handler {
	return s.requestIDMiddleware(s.mux)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if s.ready == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  "ready checker not configured",
		})
		return
	}

	if err := s.ready.Ready(ctx); err != nil {
		s.logger.Error("readyz failed",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.String("error", err.Error()),
		)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  "dependency check failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
