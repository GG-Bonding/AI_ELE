package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/config"
	"github.com/agent-experience-engine/agent-experience-engine/internal/logging"
)

func TestJSONLoggerEmitsStructuredFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logging.New(config.LogConfig{Level: "info", Format: "json"}, &buf)
	logger.Info("boot", slog.String("request_id", "req-1"), slog.String("tenant_id", "t-1"))

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal log: %v; raw=%s", err, buf.String())
	}
	if payload["msg"] != "boot" {
		t.Fatalf("msg = %v", payload["msg"])
	}
	if payload["request_id"] != "req-1" {
		t.Fatalf("request_id = %v", payload["request_id"])
	}
	if payload["tenant_id"] != "t-1" {
		t.Fatalf("tenant_id = %v", payload["tenant_id"])
	}
}
