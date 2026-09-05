package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the process configuration for Phase 0+.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Log       LogConfig       `yaml:"log"`
	LLM       LLMConfig       `yaml:"llm"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Evaluator    EvaluatorConfig    `yaml:"evaluator"`
	Retrieval    RetrievalConfig    `yaml:"retrieval"`
	SkillRuntime SkillRuntimeConfig `yaml:"skill_runtime"`
}

type ServerConfig struct {
	Addr         string        `yaml:"addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

type DatabaseConfig struct {
	URL             string        `yaml:"url"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LLMConfig configures an optional OpenAI-compatible provider for extraction.
type LLMConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

// EmbeddingConfig configures an optional OpenAI-compatible embedding provider.
type EmbeddingConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
}

// EvaluatorConfig holds quality thresholds for storing extracted candidates.
type EvaluatorConfig struct {
	ActiveMin    float64 `yaml:"active_min"`
	CandidateMin float64 `yaml:"candidate_min"`
}

// RetrievalConfig configures two-phase retrieval ranking.
type RetrievalConfig struct {
	CandidateTopK   int                `yaml:"candidate_top_k"`
	DefaultTopK     int                `yaml:"default_top_k"`
	DefaultLambda   float64            `yaml:"default_lambda"`
	ToolScopeLambda float64            `yaml:"tool_scope_lambda"`
	TypeLambda      map[string]float64 `yaml:"type_lambda"`
}

// SkillRuntimeConfig gates V3 executable Skill paths (default off).
// When disabled, V2 SkillCandidate propose/get continue; no Shadow/Live execution.
type SkillRuntimeConfig struct {
	Enabled bool `yaml:"enabled"`

	ShadowMinExecutions   int     `yaml:"shadow_min_executions"`
	ShadowMinSuccessRate  float64 `yaml:"shadow_min_success_rate"`
	SuspendWindow         int     `yaml:"suspend_window"`
	SuspendMaxFailureRate float64 `yaml:"suspend_max_failure_rate"`
	AllowMediumRiskLive   bool    `yaml:"allow_medium_risk_live"`
}

// Load reads YAML from path and applies environment overrides.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyDefaults()
	cfg.applyEnvOverrides()

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 15 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 15 * time.Second
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = 60 * time.Second
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 10
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 5
	}
	if c.Database.ConnMaxLifetime == 0 {
		c.Database.ConnMaxLifetime = 30 * time.Minute
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "gpt-4o-mini"
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = "text-embedding-3-small"
	}
	if c.Embedding.Dimensions == 0 {
		c.Embedding.Dimensions = 1536
	}
	if c.Evaluator.ActiveMin == 0 {
		c.Evaluator.ActiveMin = 0.65
	}
	if c.Evaluator.CandidateMin == 0 {
		c.Evaluator.CandidateMin = 0.4
	}
	if c.Retrieval.CandidateTopK == 0 {
		c.Retrieval.CandidateTopK = 20
	}
	if c.Retrieval.DefaultTopK == 0 {
		c.Retrieval.DefaultTopK = 20
	}
	if c.Retrieval.DefaultLambda == 0 {
		c.Retrieval.DefaultLambda = 0.02
	}
	if c.Retrieval.ToolScopeLambda == 0 {
		c.Retrieval.ToolScopeLambda = 0.05
	}
	if c.SkillRuntime.ShadowMinExecutions == 0 {
		c.SkillRuntime.ShadowMinExecutions = 5
	}
	if c.SkillRuntime.ShadowMinSuccessRate == 0 {
		c.SkillRuntime.ShadowMinSuccessRate = 0.90
	}
	if c.SkillRuntime.SuspendWindow == 0 {
		c.SkillRuntime.SuspendWindow = 10
	}
	if c.SkillRuntime.SuspendMaxFailureRate == 0 {
		c.SkillRuntime.SuspendMaxFailureRate = 0.30
	}
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("AEE_SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("AEE_DATABASE_URL"); v != "" {
		c.Database.URL = v
	}
	if v := os.Getenv("AEE_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("AEE_LLM_ENABLED"); v != "" {
		c.LLM.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("AEE_LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
	if v := os.Getenv("AEE_LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("AEE_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("AEE_EMBEDDING_ENABLED"); v != "" {
		c.Embedding.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("AEE_EMBEDDING_BASE_URL"); v != "" {
		c.Embedding.BaseURL = v
	}
	if v := os.Getenv("AEE_EMBEDDING_API_KEY"); v != "" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("AEE_EMBEDDING_MODEL"); v != "" {
		c.Embedding.Model = v
	}
	if v := os.Getenv("AEE_SKILL_RUNTIME_ENABLED"); v != "" {
		c.SkillRuntime.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
}

// Validate fails fast on missing required fields.
func (c Config) Validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("database.url is required")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be one of debug|info|warn|error, got %q", c.Log.Level)
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("log.format must be one of json|text, got %q", c.Log.Format)
	}
	if c.LLM.Enabled {
		if strings.TrimSpace(c.LLM.BaseURL) == "" {
			return fmt.Errorf("llm.base_url is required when llm.enabled is true")
		}
		if strings.TrimSpace(c.LLM.Model) == "" {
			return fmt.Errorf("llm.model is required when llm.enabled is true")
		}
	}
	if c.Embedding.Enabled {
		if strings.TrimSpace(c.Embedding.BaseURL) == "" {
			return fmt.Errorf("embedding.base_url is required when embedding.enabled is true")
		}
		if strings.TrimSpace(c.Embedding.Model) == "" {
			return fmt.Errorf("embedding.model is required when embedding.enabled is true")
		}
		if c.Embedding.Dimensions <= 0 {
			return fmt.Errorf("embedding.dimensions must be > 0")
		}
	}
	if c.Evaluator.ActiveMin < c.Evaluator.CandidateMin {
		return fmt.Errorf("evaluator.active_min must be >= evaluator.candidate_min")
	}
	return nil
}
