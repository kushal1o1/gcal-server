// gcal-svr server. Runs the same /api/remind handler locally and on Vercel
// (standalone Go server mode: Vercel builds ./cmd/server and proxy routes every
// request to it).
//
//	GCAL_SERVICE_ACCOUNT_FILE=./gcal-service-key.json API_TOKEN=dev go run ./cmd/server
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/server"
	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

func main() {
	loadDotEnv()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	warnMissingConfig()

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.Handler)
	mux.HandleFunc("/api/remind", server.Handler)
	mux.HandleFunc("/api/health", server.Handler)
	mux.HandleFunc("/api/events", server.Handler)
	mux.HandleFunc("/api/events/delete", server.Handler)
	mux.HandleFunc("/api/events/bulk-delete", server.Handler)
	mux.HandleFunc("/api/events/edit", server.Handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second, // Google Calendar list calls can be slow
		IdleTimeout:       90 * time.Second,
	}

	// SIGINT / SIGTERM start a graceful shutdown: stop accepting new
	// connections, let in-flight requests finish, then exit.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("server starting", "addr", srv.Addr, "timezone", timeparse.AppTZ().String())

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown did not finish cleanly", "error", err)
			os.Exit(1)
		}
		slog.Info("server stopped")
	}
}

// warnMissingConfig logs configuration problems at startup, so they show up in
// the logs before the first request fails.
func warnMissingConfig() {
	if os.Getenv("API_TOKEN") == "" {
		slog.Warn("API_TOKEN is not set: every authenticated request will be rejected")
	}
	if os.Getenv("GCAL_CALENDAR_ID") == "" {
		slog.Warn("GCAL_CALENDAR_ID is not set: event creation will fail")
	}
	hasKey := os.Getenv("GCAL_SERVICE_ACCOUNT_B64") != "" || os.Getenv("GCAL_SERVICE_ACCOUNT_FILE") != ""
	if _, err := os.Stat("./gcal-service-key.json"); err == nil {
		hasKey = true
	}
	if !hasKey {
		slog.Warn("no service account key configured: event creation will fail")
	}
}

// loadDotEnv sets KEY=VALUE pairs from ./gcal-svr.env or ./.env if present
// (godotenv-free, enough for local dev).
func loadDotEnv() {
	for _, p := range []string{"./gcal-svr.env", "./.env"} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
		if len(data) > 0 {
			log.Printf("loaded env from %s", p)
		}
	}
}
