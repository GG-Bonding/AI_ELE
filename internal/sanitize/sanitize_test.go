package sanitize_test

import (
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

func TestAppendUntrustedBoundary(t *testing.T) {
	t.Parallel()
	got := sanitize.AppendUntrustedBoundary("You extract experiences.")
	if !strings.Contains(got, "UNTRUSTED DATA") {
		t.Fatalf("%q", got)
	}
}
