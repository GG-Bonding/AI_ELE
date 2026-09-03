package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
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
	GetOutcome(ctx context.Context, tenantID, episodeID string) (episode.Outcome, error)
}

// ExperienceExtractor extracts candidates after an episode completes.
type ExperienceExtractor interface {
	Extract(ctx context.Context, in extractor.ExtractInput) ([]experience.Candidate, error)
}

// ExperienceService is the subset of experience.Service used by HTTP handlers.
type ExperienceService interface {
	Get(ctx context.Context, tenantID, id string) (experience.Experience, error)
	Supersede(ctx context.Context, tenantID, oldID, newID string) error
	Generalize(ctx context.Context, tenantID string, in experience.GeneralizeInput) (experience.GeneralizeResult, error)
	AutoGeneralize(ctx context.Context, tenantID string, opts experience.AutoGeneralizeOptions) (experience.AutoGeneralizeResult, error)
	GetPattern(ctx context.Context, tenantID, patternID string) (experience.Pattern, error)
	ListPatternEvidence(ctx context.Context, tenantID, patternID string) ([]experience.PatternEvidence, error)
	ApplyPatternReward(ctx context.Context, tenantID, patternID string, reward, confidence float64) (experience.Pattern, error)
	ProposeSkill(ctx context.Context, tenantID string, in experience.ProposeSkillInput) (experience.ProposeSkillResult, error)
	GetSkill(ctx context.Context, tenantID, skillID string) (experience.SkillCandidate, error)
	GetSkillByPattern(ctx context.Context, tenantID, patternID string) (experience.SkillCandidate, error)
}

// PatternRewardService applies exactly-once direct Pattern rewards (V2.1-4).
type PatternRewardService interface {
	ApplyDirectPatternReward(ctx context.Context, tenantID, patternID, idempotencyKey string, reward, confidence float64) (experience.Pattern, error)
}

// ExperienceRetriever retrieves utility-ranked experiences for a task.
type ExperienceRetriever interface {
	Retrieve(ctx context.Context, q retrieval.Query) ([]retrieval.RankedExperience, error)
}

// ExperienceStorePipeline persists extracted candidates.
type ExperienceStorePipeline interface {
	StoreCandidates(ctx context.Context, tenantID, sourceEpisodeID string, candidates []experience.Candidate) (experience.StoreCandidatesResult, error)
	StoreCandidatesWithOptions(ctx context.Context, tenantID, sourceEpisodeID string, candidates []experience.Candidate, opts experience.StoreOptions) (experience.StoreCandidatesResult, error)
}

// EpisodeLearningProcessor runs extract/store learning for completed episodes.
type EpisodeLearningProcessor interface {
	Process(ctx context.Context, in episodelearn.ProcessInput) (episodelearn.ProcessResult, error)
	Retry(ctx context.Context, tenantID string, ep episode.Episode, attempts []episode.Attempt, out episode.Outcome) (episodelearn.ProcessResult, error)
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

// ActionService records agent actions and experience→action influence links.
type ActionService interface {
	RecordAction(ctx context.Context, in action.RecordInput) (action.AgentAction, error)
	ListActions(ctx context.Context, tenantID, episodeID string) ([]action.AgentAction, error)
	LinkExperience(ctx context.Context, in action.LinkInput) (action.ExperienceActionLink, error)
	ListLinks(ctx context.Context, tenantID, episodeID string) ([]action.ExperienceActionLink, error)
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
	learning      EpisodeLearningProcessor
	contexts      ContextService
	feedbacks      FeedbackService
	actions        ActionService
	patternRewards PatternRewardService
	mux            *http.ServeMux
}

// Options configures optional server dependencies.
type Options struct {
	Episodes       EpisodeService
	Extractor      ExperienceExtractor
	Experiences    ExperienceService
	Retriever      ExperienceRetriever
	StorePipeline  ExperienceStorePipeline
	Learning       EpisodeLearningProcessor
	Contexts       ContextService
	Feedbacks      FeedbackService
	Actions        ActionService
	PatternRewards PatternRewardService
}

// New constructs an HTTP server with health and episode endpoints.
func New(logger *slog.Logger, ready ReadyChecker, opts Options) *Server {
	s := &Server{
		logger:         logger,
		ready:          ready,
		episodes:       opts.Episodes,
		extractor:      opts.Extractor,
		experiences:    opts.Experiences,
		retriever:      opts.Retriever,
		storePipeline:  opts.StorePipeline,
		learning:       opts.Learning,
		contexts:       opts.Contexts,
		feedbacks:      opts.Feedbacks,
		actions:        opts.Actions,
		patternRewards: opts.PatternRewards,
		mux:            http.NewServeMux(),
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
	s.mux.HandleFunc("POST /api/v1/episodes/{id}/learning/retry", s.handleRetryEpisodeLearning)

	s.mux.HandleFunc("GET /api/v1/experiences/{id}", s.handleGetExperience)
	s.mux.HandleFunc("POST /api/v1/experiences/search", s.handleSearchExperiences)
	s.mux.HandleFunc("POST /api/v1/experiences/{id}/supersede", s.handleSupersedeExperience)
	s.mux.HandleFunc("POST /api/v1/patterns/generalize", s.handleGeneralize)
	s.mux.HandleFunc("POST /api/v1/patterns/evolve", s.handleAutoGeneralize)
	s.mux.HandleFunc("GET /api/v1/patterns/{id}", s.handleGetPattern)
	s.mux.HandleFunc("GET /api/v1/patterns/{id}/evidence", s.handleListPatternEvidence)
	s.mux.HandleFunc("POST /api/v1/patterns/{id}/reward", s.handlePatternReward)
	s.mux.HandleFunc("POST /api/v1/patterns/{id}/skill", s.handleProposeSkill)
	s.mux.HandleFunc("GET /api/v1/patterns/{id}/skill", s.handleGetPatternSkill)
	s.mux.HandleFunc("GET /api/v1/skills/{id}", s.handleGetSkill)
	s.mux.HandleFunc("POST /api/v1/context", s.handleBuildContext)

	s.mux.HandleFunc("POST /api/v1/feedback", s.handleSubmitFeedback)
	s.mux.HandleFunc("GET /api/v1/episodes/{id}/reward", s.handleGetEpisodeReward)

	s.mux.HandleFunc("POST /api/v1/episodes/{id}/actions", s.handleRecordAction)
	s.mux.HandleFunc("GET /api/v1/episodes/{id}/actions", s.handleListActions)
	s.mux.HandleFunc("POST /api/v1/episodes/{id}/actions/{action_id}/links", s.handleLinkExperienceToAction)
	s.mux.HandleFunc("GET /api/v1/episodes/{id}/action-links", s.handleListActionLinks)
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
