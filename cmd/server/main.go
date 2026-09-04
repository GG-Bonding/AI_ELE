package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
	"github.com/agent-experience-engine/agent-experience-engine/internal/action"
	"github.com/agent-experience-engine/agent-experience-engine/internal/attribution"
	"github.com/agent-experience-engine/agent-experience-engine/internal/config"
	"github.com/agent-experience-engine/agent-experience-engine/internal/contextx"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episode"
	"github.com/agent-experience-engine/agent-experience-engine/internal/episodelearn"
	"github.com/agent-experience-engine/agent-experience-engine/internal/evolutionjob"
	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
	"github.com/agent-experience-engine/agent-experience-engine/internal/extractor"
	"github.com/agent-experience-engine/agent-experience-engine/internal/feedback"
	"github.com/agent-experience-engine/agent-experience-engine/internal/learning"
	"github.com/agent-experience-engine/agent-experience-engine/internal/logging"
	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/selector"
	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.Log, os.Stdout)
	logger.Info("starting agent experience engine", "addr", cfg.Server.Addr)

	db, err := postgres.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := postgres.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info("migrations applied")

	if *migrateOnly {
		return nil
	}

	episodeSvc := episode.NewService(postgres.NewEpisodeRepository(db))
	actionRepo := postgres.NewActionRepository(db)
	actionSvc := action.NewService(actionRepo, episodeSvc)
	experienceRepo := postgres.NewExperienceRepository(db)
	relationRepo := postgres.NewRelationRepository(db)
	patternRepo := postgres.NewPatternRepository(db)
	skillRepo := postgres.NewSkillRepository(db)
	experienceSvc := experience.NewService(experienceRepo).
		WithRelations(relationRepo).
		WithPatterns(patternRepo).
		WithSkills(skillRepo)
	usageRepo := postgres.NewUsageRepository(db)
	eventRepo := postgres.NewLearningEventRepository(db)
	learnApplier := postgres.NewLearningEventApplier(db)
	learnSvc, err := learning.NewWithEvents(usageRepo, experienceRepo, eventRepo, attribution.NewDefault(), learnApplier)
	if err != nil {
		return fmt.Errorf("init learning service: %w", err)
	}
	learnSvc = learnSvc.WithActionGraph(actionSvc, actionSvc).
		WithPatterns(patternRepo).
		WithPatternUsages(postgres.NewPatternUsageRepository(db)).
		WithPatternLearning(
			postgres.NewPatternLearningEventRepository(db),
			postgres.NewPatternLearningEventApplier(db),
		)
	evoRepo := postgres.NewEvolutionJobRepository(db)
	learnSvc = learnSvc.WithEvolutionDirty(evoRepo)
	feedbackSvc := feedback.NewServiceWithLearner(
		postgres.NewFeedbackRepository(db),
		episodeSvc,
		feedback.NewRewardEngine(nil),
		learning.FeedbackLearner{Inner: learnSvc},
	).WithActionVerifier(actionSvc)

	snapshotStore := postgres.NewContextSnapshotRepository(db)

	var llm provider.LLMProvider
	opts := httpserver.Options{
		Episodes:       episodeSvc,
		Experiences:    experienceSvc,
		Feedbacks:      feedbackSvc,
		Actions:        actionSvc,
		PatternRewards: learnSvc,
	}

	if cfg.LLM.Enabled {
		compat, err := provider.NewOpenAICompatLLM(provider.OpenAICompatConfig{
			BaseURL: cfg.LLM.BaseURL,
			APIKey:  cfg.LLM.APIKey,
			Model:   cfg.LLM.Model,
		})
		if err != nil {
			return fmt.Errorf("init llm provider: %w", err)
		}
		llm = compat
		ext, err := extractor.New(llm)
		if err != nil {
			return fmt.Errorf("init experience extractor: %w", err)
		}
		opts.Extractor = ext
		gen, err := experience.NewLLMPatternGeneralizer(llm)
		if err != nil {
			return fmt.Errorf("init llm pattern generalizer: %w", err)
		}
		experienceSvc = experienceSvc.WithGeneralizer(gen)
		opts.Experiences = experienceSvc
		logger.Info("experience extraction and LLM pattern generalizer enabled", "model", cfg.LLM.Model, "base_url", cfg.LLM.BaseURL)
	} else {
		logger.Info("experience extraction disabled")
	}

	if cfg.Embedding.Enabled {
		embedder, err := provider.NewOpenAICompatEmbedding(provider.OpenAICompatEmbeddingConfig{
			BaseURL:    cfg.Embedding.BaseURL,
			APIKey:     cfg.Embedding.APIKey,
			Model:      cfg.Embedding.Model,
			Dimensions: cfg.Embedding.Dimensions,
		})
		if err != nil {
			return fmt.Errorf("init embedding provider: %w", err)
		}
		experienceSvc = experienceSvc.WithEmbedder(embedder)
		storeCfg := experience.StorePipelineConfig{
			ActiveMin:    cfg.Evaluator.ActiveMin,
			CandidateMin: cfg.Evaluator.CandidateMin,
		}
		if llm != nil {
			storeCfg.SemanticDedup = experience.SemanticDedupConfig{
				Judge: experience.NewHybridDedupJudge(llm),
			}
			logger.Info("structured semantic dedup judge enabled")
		}
		pipeline, err := experience.NewStorePipeline(experienceSvc, embedder, storeCfg)
		if err != nil {
			return fmt.Errorf("init store pipeline: %w", err)
		}
		rankCfg := rankConfigFrom(cfg.Retrieval)
		retriever, err := retrieval.New(experienceSvc, embedder, rankCfg)
		if err != nil {
			return fmt.Errorf("init retriever: %w", err)
		}
		contextSvc, err := contextx.NewServiceWithUsage(
			retriever,
			selector.New(selector.DefaultConfig()),
			contextx.New(contextx.DefaultConfig()),
			learning.Recorder{Inner: learnSvc},
		)
		if err != nil {
			return fmt.Errorf("init context service: %w", err)
		}
		contextSvc = contextSvc.WithConflicts(experienceSvc).WithSnapshots(snapshotStore)
		if patternRetriever, err := retrieval.NewPatternRetriever(patternRepo, embedder, rankCfg); err != nil {
			return fmt.Errorf("init pattern retriever: %w", err)
		} else {
			contextSvc = contextSvc.WithPatterns(patternRetriever)
		}
		actionSvc = actionSvc.WithContexts(action.SnapshotFunc(func(ctx context.Context, tenantID, contextID string) (action.ContextSnapshot, error) {
			snap, err := contextSvc.GetSnapshot(ctx, tenantID, contextID)
			if err != nil {
				if errors.Is(err, contextx.ErrSnapshotNotFound) {
					return action.ContextSnapshot{}, action.ErrContextNotFound
				}
				return action.ContextSnapshot{}, err
			}
			return action.ContextSnapshot{
				ID:            snap.ID,
				TenantID:      snap.TenantID,
				EpisodeID:     snap.EpisodeID,
				ExperienceIDs: snap.ExperienceIDs,
				PatternIDs:    snap.PatternIDs,
			}, nil
		}))
		opts.Actions = actionSvc
		opts.StorePipeline = pipeline
		opts.Retriever = retriever
		opts.Contexts = contextSvc
		logger.Info("experience retrieval enabled", "model", cfg.Embedding.Model, "dimensions", cfg.Embedding.Dimensions)
	} else {
		logger.Info("experience retrieval disabled")
	}

	if opts.Extractor != nil && opts.StorePipeline != nil {
		learnJobs := postgres.NewEpisodeLearningRepository(db)
		processor, err := episodelearn.NewProcessor(opts.Extractor, opts.StorePipeline, learnJobs)
		if err != nil {
			return fmt.Errorf("init episode learning processor: %w", err)
		}
		if n, err := processor.RecoverStaleJobs(context.Background()); err != nil {
			return fmt.Errorf("recover stale learning jobs: %w", err)
		} else if n > 0 {
			logger.Info("recovered stale episode learning jobs", "count", n)
		}
		opts.Learning = processor
		logger.Info("episode learning processor enabled")
	}

	evoProc, err := evolutionjob.NewProcessor(experienceSvc, evoRepo)
	if err != nil {
		return fmt.Errorf("init evolution processor: %w", err)
	}
	evoProc = evoProc.WithLogger(logger)
	if n, err := evoProc.RecoverStaleJobs(context.Background()); err != nil {
		return fmt.Errorf("recover stale evolution jobs: %w", err)
	} else if n > 0 {
		logger.Info("recovered stale evolution jobs", "count", n)
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go evoProc.RunLoop(workerCtx, evolutionjob.DefaultSweepInterval)
	logger.Info("evolution worker enabled", "interval", evolutionjob.DefaultSweepInterval.String())

	srv := httpserver.New(logger, httpserver.DBReady{DB: db}, opts)
	httpServer := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      srv.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.Server.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

func rankConfigFrom(cfg config.RetrievalConfig) retrieval.RankConfig {
	rc := retrieval.DefaultRankConfig()
	rc.CandidateTopK = cfg.CandidateTopK
	rc.DefaultTopK = cfg.DefaultTopK
	rc.DefaultLambda = cfg.DefaultLambda
	rc.ToolScopeLambda = cfg.ToolScopeLambda
	if len(cfg.TypeLambda) > 0 {
		rc.TypeLambda = make(map[experience.Type]float64, len(cfg.TypeLambda))
		for k, v := range cfg.TypeLambda {
			rc.TypeLambda[experience.Type(k)] = v
		}
	}
	return rc
}
