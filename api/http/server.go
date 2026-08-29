package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
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

// EpisodeService is the subset of episode.Service used by HTTP handlers.
type EpisodeService interface {
	CreateEpisode(ctx context.Context, in episode.CreateEpisodeInput) (episode.Episode, error)
	GetEpisode(ctx context.Context, tenantID, id string) (episode.Episode, error)
	AddAttempt(ctx context.Context, in episode.AddAttemptInput) (episode.Attempt, error)
	ListAttempts(ctx context.Context, tenantID, episodeID string) ([]episode.Attempt, error)
	CompleteEpisode(ctx context.Context, in episode.CompleteEpisodeInput) (episode.Episode, episode.Outcome, error)
}

// ExperienceExtractor extracts candidates after an episode completes.
type ExperienceExtractor interface {
	Extract(ctx context.Context, in extractor.ExtractInput) ([]experience.Candidate, error)
}

// ExperienceService is the subset of experience.Service used by HTTP handlers.
type ExperienceService interface {
	Get(ctx context.Context, tenantID, id string) (experience.Experience, error)
}

// ExperienceRetriever retrieves utility-ranked experiences for a task.
type ExperienceRetriever interface {
	Retrieve(ctx context.Context, q retrieval.Query) ([]retrieval.RankedExperience, error)
}

// ExperienceStorePipeline persists extracted candidates.
type ExperienceStorePipeline interface {
	StoreCandidates(ctx context.Context, tenantID, sourceEpisodeID string, candidates []experience.Candidate) (experience.StoreCandidatesResult, error)
}

// ContextService builds agent context from selected experiences.
type ContextService interface {
	BuildContext(ctx context.Context, req contextx.Request) (contextx.Response, error)
}

// FeedbackService collects and aggregates episode feedback.
type FeedbackService interface {
	Submit(ctx context.Context, in feedback.SubmitInput) (feedback.SubmitResult, error)
	GetEpisodeReward(ctx context.Context, tenantID, episodeID string) (feedback.EpisodeReward, []feedback.Feedback, error)
}

// Server is the HTTP API surface.
type Server struct {
	logger        *slog.Logger
	ready         ReadyChecker
	episodes      EpisodeService
	extractor     ExperienceExtractor
	experiences   ExperienceService
	retriever     ExperienceRetriever
	storePipeline ExperienceStorePipeline
	contexts      ContextService
	feedbacks     FeedbackService
	mux           *http.ServeMux
}

// Options configures optional server dependencies.
type Options struct {
	Episodes      EpisodeService
	Extractor     ExperienceExtractor
	Experiences   ExperienceService
	Retriever     ExperienceRetriever
	StorePipeline ExperienceStorePipeline
	Contexts      ContextService
	Feedbacks     FeedbackService
}

// New constructs an HTTP server with health and episode endpoints.
func New(logger *slog.Logger, ready ReadyChecker, opts Options) *Server {
	s := &Server{
		logger:        logger,
		ready:         ready,
		episodes:      opts.Episodes,
		extractor:     opts.Extractor,
		experiences:   opts.Experiences,
		retriever:     opts.Retriever,
		storePipeline: opts.StorePipeline,
		contexts:      opts.Contexts,
		feedbacks:     opts.Feedbacks,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	s.mux.HandleFunc("POST /api/v1/episodes", s.handleCreateEpisode)
	s.mux.HandleFunc("GET /api/v1/episodes/{id}", s.handleGetEpisode)
	s.mux.HandleFunc("POST /api/v1/episodes/{id}/attempts", s.handleAddAttempt)
	s.mux.HandleFunc("POST /api/v1/episodes/{id}/outcome", s.handleCompleteOutcome)

	s.mux.HandleFunc("GET /api/v1/experiences/{id}", s.handleGetExperience)
	s.mux.HandleFunc("POST /api/v1/experiences/search", s.handleSearchExperiences)
	s.mux.HandleFunc("POST /api/v1/context", s.handleBuildContext)

	s.mux.HandleFunc("POST /api/v1/feedback", s.handleSubmitFeedback)
	s.mux.HandleFunc("GET /api/v1/episodes/{id}/reward", s.handleGetEpisodeReward)
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
