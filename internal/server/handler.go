package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/apiauth"
	"github.com/kushal1o1/gcal-svr/internal/apperr"
	"github.com/kushal1o1/gcal-svr/internal/gcal"
	"github.com/kushal1o1/gcal-svr/internal/remind"
	"github.com/kushal1o1/gcal-svr/internal/timing"
)

// Handler serves the API for both the standalone Vercel Go server and the
// local dev server. Every response uses a uniform envelope:
//
//	{"success": true, "message": "...", "data": {...}}
//
//	GET  /api/health          -> health probe
//	GET  /api/remind          -> health probe
//	POST /api/remind          -> {text, create}
//	GET  /api/events          -> list (mirrors `gcal list`)
//	POST/DELETE /api/events/delete -> {id, delete_series} (mirrors `gcal delete <id> [-A]`)
//	POST /api/events/bulk-delete   -> {kind: "recurring"|"critical"} (mirrors `gcal delete -A|-C`)
//	POST /api/events/edit         -> {id, use_default, reminders} (edit reminder config)
//
// Mutating and data endpoints require `Authorization: Bearer <API_TOKEN>`.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		handleRoot(w, r)

	case "/api/health":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeOK(w, http.StatusOK, "ok", map[string]interface{}{"service": "gcal-svr"})

	case "/api/remind":
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			writeOK(w, http.StatusOK, "ok", map[string]interface{}{"service": "gcal-svr"})
			return
		}
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use POST)")
			return
		}
		handleRemind(w, r)

	case "/api/events":
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use GET)")
			return
		}
		handleListEvents(w, r)

	case "/api/events/delete":
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use POST)")
			return
		}
		handleDeleteEvent(w, r)

	case "/api/events/bulk-delete":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use POST)")
			return
		}
		handleBulkDelete(w, r)

	case "/api/events/edit":
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed (use POST)")
			return
		}
		handleEditEvent(w, r)

	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

func handleRemind(w http.ResponseWriter, r *http.Request) {
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
	total := time.Now()
	defer timing.Since("remind_total", total)

	// Calendar service is only needed when actually creating an event, so a
	// preview (create=false) never fails on missing credentials.
	var svc *gcal.Service
	if req.Create {
		t := time.Now()
		var err error
		svc, err = gcal.NewService(ctx)
		timing.Since("gcal_service_init", t)
		if err != nil {
			writeError(w, r, err)
			return
		}
	}

	resp, err := remind.Run(ctx, &req, svc, time.Time{})
	if err != nil {
		writeError(w, r, err)
		return
	}

	message := "Preview ready"
	if resp.Created {
		message = "Event created"
		defaultListCache.Clear()
	}
	writeOK(w, http.StatusOK, message, resp)
}

type reminderItem struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

type eventJSON struct {
	ID                string         `json:"id"`
	MasterID          string         `json:"master_id"`
	Summary           string         `json:"summary"`
	Critical          bool           `json:"critical"`
	Recurring         bool           `json:"recurring"`
	StartsAt          string         `json:"starts_at,omitempty"`
	StartsAtISO       string         `json:"starts_at_iso,omitempty"`
	Time              string         `json:"time,omitempty"`
	RecurrenceDisplay string         `json:"recurrence_display,omitempty"`
	Reminders         []reminderItem `json:"reminders,omitempty"`
}

func handleListEvents(w http.ResponseWriter, r *http.Request) {
	if !apiauth.Valid(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	opts := gcal.ListOptions{
		Limit:         -1,
		RecurringOnly: boolQuery(q.Get("recurring_only")),
		CriticalOnly:  boolQuery(q.Get("critical_only")),
		NormalOnly:    boolQuery(q.Get("normal_only")),
	}
	if opts.CriticalOnly && opts.NormalOnly {
		writeErr(w, http.StatusBadRequest, "cannot use critical_only and normal_only together")
		return
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if n > gcal.MaxListLimit {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("limit too high (max %d)", gcal.MaxListLimit))
			return
		}
		opts.Limit = n
	}

	ctx := r.Context()
	svc, err := gcal.NewService(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}

	cacheKey := listEventsKey(opts)
	if body, status, ok := defaultListCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}

	events, hasMore, err := svc.ListEvents(ctx, opts)
	if err != nil {
		writeError(w, r, err)
		return
	}

	items := make([]eventJSON, 0, len(events))
	for _, e := range events {
		items = append(items, eventJSONFrom(e))
	}

	message := fmt.Sprintf("%d events found", len(items))
	if len(items) == 0 {
		message = "No events found"
	}
	body := marshalEnvelope(http.StatusOK, message, map[string]interface{}{
		"count":    len(items),
		"has_more": hasMore,
		"events":   items,
	})
	defaultListCache.Put(cacheKey, body, http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func eventJSONFrom(e gcal.EventItem) eventJSON {
	out := eventJSON{
		ID:                e.ID,
		MasterID:          e.MasterID,
		Summary:           e.Summary,
		Critical:          e.Critical,
		Recurring:         e.Recurring,
		RecurrenceDisplay: e.RecurrenceDisplay,
	}
	if len(e.Reminders) > 0 {
		out.Reminders = make([]reminderItem, 0, len(e.Reminders))
		for _, r := range e.Reminders {
			out.Reminders = append(out.Reminders, reminderItem{Method: r.Method, Minutes: r.Minutes})
		}
	}
	if !e.Start.IsZero() {
		out.StartsAt = e.Start.Format("2006-01-02 3:04 PM")
		out.StartsAtISO = e.Start.Format(time.RFC3339)
		out.Time = e.Start.Format("3:04 PM, 02-Jan")
	}
	if e.StartDate != "" {
		out.StartsAt = e.StartDate
		out.Time = e.StartDate
	}
	return out
}

type deleteRequest struct {
	ID           string `json:"id"`
	DeleteSeries bool   `json:"delete_series"`
}

func handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if !apiauth.Valid(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req deleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ID == "" {
		writeErr(w, http.StatusBadRequest, "id is required")
		return
	}

	ctx := r.Context()
	svc, err := gcal.NewService(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := svc.DeleteEvent(ctx, req.ID); err != nil {
		if errors.Is(err, gcal.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, r, err)
		return
	}

	message := "Event deleted"
	if req.DeleteSeries {
		message = "Recurring series deleted"
	}
	defaultListCache.Clear()
	writeOK(w, http.StatusOK, message, map[string]interface{}{"deleted": true, "id": req.ID})
}

type bulkDeleteRequest struct {
	Kind string `json:"kind"`
}

type bulkItemJSON struct {
	Summary string `json:"summary"`
	Series  bool   `json:"series"`
	OK      bool   `json:"ok"`
	Err     string `json:"error,omitempty"`
}

func handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	if !apiauth.Valid(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req bulkDeleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx := r.Context()
	svc, err := gcal.NewService(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}

	var items []gcal.BulkDeleteItem
	kind := req.Kind
	switch kind {
	case "recurring":
		items, err = svc.BulkDeleteRecurring(ctx)
	case "critical":
		items, err = svc.BulkDeleteCritical(ctx)
	default:
		writeErr(w, http.StatusBadRequest, `kind must be "recurring" or "critical"`)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}

	deleted, failed := 0, 0
	result := make([]bulkItemJSON, 0, len(items))
	for _, it := range items {
		if it.OK {
			deleted++
		} else {
			failed++
		}
		result = append(result, bulkItemJSON{Summary: it.Summary, Series: it.Series, OK: it.OK, Err: it.Err})
	}

	message := fmt.Sprintf("Deleted %d %s events", deleted, kind)
	switch {
	case deleted == 0 && failed == 0:
		message = fmt.Sprintf("No %s events found", kind)
	case failed > 0:
		message += fmt.Sprintf(" (%d failed)", failed)
	}
	if deleted > 0 {
		defaultListCache.Clear()
	}
	writeOK(w, http.StatusOK, message, map[string]interface{}{
		"deleted": deleted,
		"failed":  failed,
		"items":   result,
	})
}

type editRequest struct {
	ID         string         `json:"id"`
	UseDefault bool           `json:"use_default"`
	Reminders  []reminderItem `json:"reminders"`
}

func handleEditEvent(w http.ResponseWriter, r *http.Request) {
	if !apiauth.Valid(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req editRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ID == "" {
		writeErr(w, http.StatusBadRequest, "id is required")
		return
	}

	patch := gcal.ReminderPatch{UseDefault: req.UseDefault}
	for _, r := range req.Reminders {
		if r.Minutes < 0 {
			writeErr(w, http.StatusBadRequest, "reminder minutes must be >= 0")
			return
		}
		patch.Reminders = append(patch.Reminders, gcal.ReminderSetting{Method: r.Method, Minutes: r.Minutes})
	}

	ctx := r.Context()
	svc, err := gcal.NewService(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if err := svc.EditReminders(ctx, req.ID, patch); err != nil {
		if errors.Is(err, gcal.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, r, err)
		return
	}

	defaultListCache.Clear()
	writeOK(w, http.StatusOK, "Reminders updated", map[string]interface{}{"updated": true, "id": req.ID})
}

func boolQuery(v string) bool {
	switch v {
	case "1", "true", "yes":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, status int, message string, data interface{}) {
	writeJSON(w, status, map[string]interface{}{"success": true, "message": message, "data": data})
}

// writeError sends the client only the safe message and code from apperr, and
// logs the real cause together with the request id so it can be found later.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ae := apperr.From(err)
	id := RequestID(r.Context())
	if ae.Cause != nil {
		level := slog.LevelInfo
		if ae.Status >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "request error",
			"request_id", id, "status", ae.Status, "code", ae.Code, "cause", ae.Cause.Error())
	}
	writeJSON(w, ae.Status, map[string]any{
		"success":    false,
		"message":    ae.Message,
		"code":       ae.Code,
		"request_id": id,
		"data":       nil,
	})
}

func writeErr(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"success": false, "message": message, "data": nil})
}

// marshalEnvelope renders the standard success envelope without touching the
// writer, so /api/events responses can be cached verbatim.
func marshalEnvelope(status int, message string, data interface{}) []byte {
	body, err := json.Marshal(map[string]interface{}{"success": true, "message": message, "data": data})
	if err != nil {
		return []byte(`{"success":true,"message":"","data":null}`)
	}
	return body
}

// listEventsKey canonicalises the list options into a cache key.
func listEventsKey(opts gcal.ListOptions) string {
	return fmt.Sprintf("l=%d,r=%v,c=%v,n=%v", opts.Limit, opts.RecurringOnly, opts.CriticalOnly, opts.NormalOnly)
}
