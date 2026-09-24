// Package gcal talks to the Google Calendar API with the same service-account
// setup as the original gcal CLI.
package gcal

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"golang.org/x/oauth2/google"
)

const scope = "https://www.googleapis.com/auth/calendar"

// Service wraps an authenticated Google Calendar client plus the target
// calendar (shared with the service account, same as the CLI's CALENDAR_ID).
type Service struct {
	Svc        *calendar.Service
	CalendarID string
}

// NewService builds a Service from environment configuration:
//
//	GCAL_SERVICE_ACCOUNT_B64  base64-encoded service-account JSON key (Vercel)
//	GCAL_SERVICE_ACCOUNT_FILE path to the key file (local dev, optional)
//	GCAL_CALENDAR_ID          the calendar to write into
//
// The key is also auto-discovered as ./gcal-service-key.json for local runs.
func NewService(ctx context.Context) (*Service, error) {
	keyData, err := loadServiceAccountKey()
	if err != nil {
		return nil, err
	}

	creds, err := google.CredentialsFromJSON(ctx, keyData, scope)
	if err != nil {
		return nil, fmt.Errorf("parse service account key: %w", err)
	}

	svc, err := calendar.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	calendarID := os.Getenv("GCAL_CALENDAR_ID")
	if calendarID == "" {
		return nil, fmt.Errorf("GCAL_CALENDAR_ID not set")
	}

	return &Service{Svc: svc, CalendarID: calendarID}, nil
}

func loadServiceAccountKey() ([]byte, error) {
	if b64 := os.Getenv("GCAL_SERVICE_ACCOUNT_B64"); b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("GCAL_SERVICE_ACCOUNT_B64 is not valid base64: %w", err)
		}
		return data, nil
	}

	paths := []string{}
	if f := os.Getenv("GCAL_SERVICE_ACCOUNT_FILE"); f != "" {
		paths = append(paths, f)
	}
	paths = append(paths, "gcal-service-key.json")

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("no service account key found (set GCAL_SERVICE_ACCOUNT_B64 or GCAL_SERVICE_ACCOUNT_FILE)")
}