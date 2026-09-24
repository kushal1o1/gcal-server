// Local dev server exposing the same /api/remind handler as Vercel, so you can
// curl-test the full pipeline without deploying.
//
//	GCAL_SERVICE_ACCOUNT_FILE=./gcal-service-key.json API_TOKEN=dev go run ./cmd/server
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/kushal1o1/gcal-svr/api"
)

func main() {
	loadDotEnv()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/remind", api.Handler)
	mux.HandleFunc("/api/health", api.Handler)

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	addr = ":" + addr

	log.Printf("gcal-svr local server on %s", addr)
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