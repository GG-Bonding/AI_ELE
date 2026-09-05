package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/config"
)

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  addr: ":9090"
database:
  url: "postgres://aee:aee@localhost:5432/aee?sslmode=disable"
log:
  level: debug
  format: json
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Addr != ":9090" {
		t.Fatalf("addr = %q, want :9090", cfg.Server.Addr)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("level = %q, want debug", cfg.Log.Level)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  addr: ":8080"
database:
  url: ""
log:
  level: info
  format: json
`)

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing database.url")
	}
}

func TestEnvOverridesDatabaseURL(t *testing.T) {
	path := writeTempConfig(t, `
database:
  url: "postgres://old/old"
log:
  level: info
  format: json
`)

	t.Setenv("AEE_DATABASE_URL", "postgres://env/env")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "postgres://env/env" {
		t.Fatalf("url = %q, want env override", cfg.Database.URL)
	}
}

func TestSkillRuntimeDefaultsDisabled(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
database:
  url: "postgres://aee:aee@localhost:5432/aee?sslmode=disable"
log:
  level: info
  format: json
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SkillRuntime.Enabled {
		t.Fatal("skill_runtime.enabled must default to false")
	}
}

func TestSkillRuntimeEnvOverride(t *testing.T) {
	path := writeTempConfig(t, `
database:
  url: "postgres://aee:aee@localhost:5432/aee?sslmode=disable"
log:
  level: info
  format: json
skill_runtime:
  enabled: false
`)
	t.Setenv("AEE_SKILL_RUNTIME_ENABLED", "true")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SkillRuntime.Enabled {
		t.Fatal("expected AEE_SKILL_RUNTIME_ENABLED to enable skill runtime")
	}
}

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
