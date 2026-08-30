package sanitize_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/internal/sanitize"
)

func TestTraceRedactsSecrets(t *testing.T) {
	t.Parallel()
	in := `Authorization: Bearer abc.def.ghi
api_key=supersecret
password: hunter2
{"access_token":"tok_123"}`
	out := sanitize.Trace(in, sanitize.DefaultConfig())
	for _, bad := range []string{"abc.def.ghi", "supersecret", "hunter2", "tok_123"} {
		if strings.Contains(out, bad) {
			t.Fatalf("secret leaked in %q", out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") && !strings.Contains(out, "REDACTED_TOKEN") {
		t.Fatalf("expected redaction markers: %q", out)
	}
}

func TestJSONBytesRedactsSensitiveKeys(t *testing.T) {
	t.Parallel()
	out := sanitize.JSONBytes([]byte(`{"access_token":"tok_123","ok":true}`), sanitize.DefaultConfig())
	if !json.Valid(out) {
		t.Fatalf("invalid json: %s", out)
	}
	if strings.Contains(string(out), "tok_123") {
		t.Fatalf("access_token not redacted: %s", out)
	}
	if !strings.Contains(string(out), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in %s", out)
	}
}

func TestAppendUntrustedBoundary(t *testing.T) {
	t.Parallel()
	got := sanitize.AppendUntrustedBoundary("You extract experiences.")
	if !strings.Contains(got, "UNTRUSTED DATA") {
		t.Fatalf("%q", got)
	}
}
