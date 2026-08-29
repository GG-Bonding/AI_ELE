package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/agent-experience-engine/agent-experience-engine/api/http"
)

type stubReady struct {
	err error
}

func (s stubReady) Ready(context.Context) error {
	return s.err
}

func TestHealthzOK(t *testing.T) {
	t.Parallel()

	srv := httpserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)), stubReady{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertJSONStatus(t, rec.Body.Bytes(), "ok")
}

func TestReadyzOK(t *testing.T) {
	t.Parallel()

	srv := httpserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)), stubReady{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertJSONStatus(t, rec.Body.Bytes(), "ready")
}

func TestReadyzUnavailable(t *testing.T) {
	t.Parallel()

	srv := httpserver.New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		stubReady{err: errors.New("db down")},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	assertJSONStatus(t, rec.Body.Bytes(), "not_ready")
}

func TestRequestIDHeaderPropagated(t *testing.T) {
	t.Parallel()

	srv := httpserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)), stubReady{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "fixed-id")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "fixed-id" {
		t.Fatalf("X-Request-ID = %q, want fixed-id", got)
	}
}

func assertJSONStatus(t *testing.T, body []byte, want string) {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != want {
		t.Fatalf("status = %q, want %q", payload["status"], want)
	}
}
