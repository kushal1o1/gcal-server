// Package apiauth provides a minimal shared-secret check for the API.
package apiauth

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

// Valid reports whether the request carries a Bearer token matching API_TOKEN.
func Valid(r *http.Request) bool {
	expected := os.Getenv("API_TOKEN")
	if expected == "" {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
