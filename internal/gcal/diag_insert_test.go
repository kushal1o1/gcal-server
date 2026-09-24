package gcal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"golang.org/x/oauth2/google"
)

type logTransport struct{}

func (logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost {
		b, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(b))
		if !bytes.Contains(b, []byte(`"key"`)) {
			fmt.Printf("BODY: %s\n", string(b))
		}
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestDiagInsertRaw(t *testing.T) {
	ctx := context.Background()
	keyData, _ := os.ReadFile("/home/kushal1o1/playground/kushal1o1/gcal/gcal-service-key.json")
	creds, err := google.CredentialsFromJSON(ctx, keyData, scope)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := calendar.NewService(ctx, option.WithCredentials(creds),
		option.WithHTTPClient(&http.Client{Transport: logTransport{}}))
	if err != nil {
		t.Fatal(err)
	}
	calID := "9c1e89c416328ccafc5e708b97694384557d6f08b05981ab543f2e8ae69ab3cd@group.calendar.google.com"

	ev := &calendar.Event{
		Summary: "[gcal-svr diag] reminders probe",
		Start:   &calendar.EventDateTime{DateTime: "2026-09-30T09:00:00+05:45", TimeZone: "Asia/Kathmandu"},
		End:     &calendar.EventDateTime{DateTime: "2026-09-30T10:00:00+05:45", TimeZone: "Asia/Kathmandu"},
		Reminders: &calendar.EventReminders{
			UseDefault:      false,
			ForceSendFields: []string{"UseDefault"},
			Overrides:       []*calendar.EventReminder{{Method: "popup", Minutes: 0}},
		},
	}
	created, err := svc.Events.Insert(calID, ev).Context(ctx).Do()
	if err != nil {
		fmt.Println("INSERT ERROR:", err)
		return
	}
	fmt.Println("INSERT OK id:", created.Id)
	_ = svc.Events.Delete(calID, created.Id).Context(ctx).Do()
}