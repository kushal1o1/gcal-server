package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorDoesNotLeakInternalText(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	writeError(rec, req, errors.New("secret-token-xyz database password"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-token-xyz") || strings.Contains(body, "password") {
		t.Fatalf("response leaks internal error: %s", body)
	}
	if !strings.Contains(body, `"code":"internal"`) {
		t.Fatalf("response missing code: %s", body)
	}
}

func TestMiddlewareRecoversPanic(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom with internal details")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/remind", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Fatalf("panic value leaked to client: %s", rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID header missing")
	}
}

func TestSanitizeRequestID(t *testing.T) {
	if got := sanitizeRequestID("abc-123_X"); got != "abc-123_X" {
		t.Fatalf("valid id rejected: %q", got)
	}
	for _, bad := range []string{"", "has space", "new\nline", "a/b", strings.Repeat("a", 65)} {
		if got := sanitizeRequestID(bad); got != "" {
			t.Fatalf("unsafe id %q accepted as %q", bad, got)
		}
	}
}
