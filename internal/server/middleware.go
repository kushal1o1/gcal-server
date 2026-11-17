package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type ctxKey struct{}

// RequestID returns the request id set by Middleware, or "" outside a request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Middleware adds a request id (echoed in X-Request-ID), recovers panics into a
// generic 500, and logs one line per request. It never logs query strings or
// headers, so tokens cannot end up in the logs.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, id))

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic", "request_id", id, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
				if !rec.wrote {
					writeJSON(rec, http.StatusInternalServerError, map[string]any{
						"success":    false,
						"message":    "internal server error",
						"code":       "internal",
						"request_id": id,
						"data":       nil,
					})
				}
				rec.status = http.StatusInternalServerError
			}
			slog.Info("request",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// sanitizeRequestID accepts a caller-supplied id only if it is short and plain,
// so a client cannot inject arbitrary text into the logs.
func sanitizeRequestID(v string) string {
	if v == "" || len(v) > 64 {
		return ""
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return ""
		}
	}
	return v
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
