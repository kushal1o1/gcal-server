// gcal-svr server. Runs the same /api/remind handler locally and on Vercel
// (standalone Go server mode: Vercel builds ./cmd/server and proxy routes every
// request to it).
//
//	GCAL_SERVICE_ACCOUNT_FILE=./gcal-service-key.json API_TOKEN=dev go run ./cmd/server
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/kushal1o1/gcal-svr/internal/server"
)

func main() {
	loadDotEnv()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/remind", server.Handler)
	mux.HandleFunc("/api/health", server.Handler)

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	addr = ":" + addr

	log.Printf("gcal-svr server on %s", addr)
	log.Printf("  POST /api/remind  {text,create}  (Authorization: Bearer <API_TOKEN>)")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
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
