// Package apperr separates what an API client sees from what the operator logs.
//
// Every error that leaves the server carries a safe, client-facing Message and
// a stable Code.
package apperr

import (
	"errors"

	"google.golang.org/api/googleapi"

	"github.com/kushal1o1/gcal-svr/internal/gcal"
)

// Error is an error that is safe to return to API clients.
type Error struct {
	Status  int    // HTTP status code
	Code    string // stable machine-readable code, e.g. "unprocessable"
	Message string // safe to show the client
	Cause   error  // for logs only; never sent to the client
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// BadRequest is a malformed request (400).
func BadRequest(msg string) *Error {
	return &Error{Status: 400, Code: "bad_request", Message: msg}
}

// NotFound is a missing resource (404).
func NotFound(msg string) *Error {
	return &Error{Status: 404, Code: "not_found", Message: msg}
}

// Unprocessable is input the server could not understand (422). The message
// is written for the user, so it may be shown as-is.
func Unprocessable(msg string) *Error {
	return &Error{Status: 422, Code: "unprocessable", Message: msg}
}

// Internal is an unexpected server-side failure (500). The client only gets a
// generic message; the cause goes to the log.
func Internal(cause error) *Error {
	return &Error{Status: 500, Code: "internal", Message: "internal server error", Cause: cause}
}

// From converts any error into an *Error. Errors that are already *Error are
// returned unchanged, Google API errors are mapped to a status and a generic
// message, and everything else becomes Internal.
func From(err error) *Error {
	if err == nil {
		return Internal(errors.New("nil error passed to apperr.From"))
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, gcal.ErrNotFound) {
		return &Error{Status: 404, Code: "not_found", Message: "event not found", Cause: err}
	}
	var g *googleapi.Error
	if errors.As(err, &g) {
		return fromGoogle(g, err)
	}
	return Internal(err)
}

// fromGoogle maps a Google Calendar API failure to a client-safe error. Google's
// own text (which can include request details) is kept in Cause only.
func fromGoogle(g *googleapi.Error, cause error) *Error {
	switch {
	case g.Code == 404:
		return &Error{Status: 404, Code: "not_found", Message: "event not found", Cause: cause}
	case g.Code == 429:
		return &Error{Status: 503, Code: "calendar_busy", Message: "the calendar is busy, please try again shortly", Cause: cause}
	case g.Code == 401 || g.Code == 403:
		return &Error{Status: 502, Code: "calendar_auth", Message: "calendar access failed", Cause: cause}
	case g.Code >= 500:
		return &Error{Status: 502, Code: "calendar_unavailable", Message: "the calendar service is unavailable, please try again", Cause: cause}
	default:
		return &Error{Status: 502, Code: "calendar_error", Message: "the calendar rejected the request", Cause: cause}
	}
}
