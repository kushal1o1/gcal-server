package apperr

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"

	"github.com/kushal1o1/gcal-svr/internal/gcal"
)

func TestFromMapsGoogleErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", &googleapi.Error{Code: 404}, 404, "not_found"},
		{"rate limited", &googleapi.Error{Code: 429}, 503, "calendar_busy"},
		{"forbidden", &googleapi.Error{Code: 403}, 502, "calendar_auth"},
		{"unauthorized", &googleapi.Error{Code: 401}, 502, "calendar_auth"},
		{"server error", &googleapi.Error{Code: 503}, 502, "calendar_unavailable"},
		{"bad request", &googleapi.Error{Code: 400}, 502, "calendar_error"},
		{"wrapped google error", fmt.Errorf("fetch events: %w", &googleapi.Error{Code: 500}), 502, "calendar_unavailable"},
		{"gcal not found", gcal.ErrNotFound, 404, "not_found"},
		{"plain error", errors.New("boom"), 500, "internal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := From(c.err)
			if got.Status != c.status || got.Code != c.code {
				t.Fatalf("From(%v) = (%d,%s), want (%d,%s)", c.err, got.Status, got.Code, c.status, c.code)
			}
		})
	}
}

// The client message must never contain the raw upstream or internal text.
func TestFromHidesCauseFromMessage(t *testing.T) {
	secret := "Insufficient Permission: service account token abc123"
	got := From(&googleapi.Error{Code: 403, Message: secret})
	if strings.Contains(got.Message, "abc123") || strings.Contains(got.Message, "Permission") {
		t.Fatalf("client message leaks upstream text: %q", got.Message)
	}
	if !errors.Is(got, got.Cause) || got.Cause == nil {
		t.Fatal("cause must be kept for logging")
	}
}

func TestFromKeepsExistingAppError(t *testing.T) {
	orig := Unprocessable("could not extract a summary")
	if got := From(fmt.Errorf("wrap: %w", orig)); got != orig {
		t.Fatalf("From should return the existing *Error unchanged")
	}
}
