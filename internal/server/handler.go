package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/apiauth"
	"github.com/kushal1o1/gcal-svr/internal/gcal"
	"github.com/kushal1o1/gcal-svr/internal/remind"
)

// Handler serves /api/remind (and /api/health) for both the standalone Vercel
// Go server and the local dev server.
//
//	GET  /api/remind          -> health/probe JSON
//	POST /api/remind          -> {text, create} with `Authorization: Bearer <API_TOKEN>`
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "service": "gcal-svr"})
		return
	}

	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use POST)")
		return
	}

	if !apiauth.Valid(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req remind.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx := r.Context()

	// Calendar service is only needed when actually creating an event, so a
	// preview (create=false) never fails on missing credentials.
	var svc *gcal.Service
	if req.Create {
		var err error
		svc, err = gcal.NewService(ctx)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	resp, err := remind.Run(ctx, &req, svc, time.Time{})
	if err != nil {
		if apiErr, ok := err.(*remind.Error); ok {
			writeErr(w, apiErr.Status, apiErr.Message)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"error": message})
}
