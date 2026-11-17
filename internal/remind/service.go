// Package remind orchestrates the full pipeline: text -> intent -> schedule
// -> (optionally) create the Google Calendar event.
package remind

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/apperr"
	"github.com/kushal1o1/gcal-svr/internal/gcal"
	"github.com/kushal1o1/gcal-svr/internal/nlu"
	"github.com/kushal1o1/gcal-svr/internal/timeparse"
	"github.com/kushal1o1/gcal-svr/internal/timing"
)

// Request from the mobile app. Create=false returns a preview without writing
// anything to the calendar.
type Request struct {
	Text   string `json:"text"`
	Create bool   `json:"create"`
}

// Response returned to the app.
type Response struct {
	Ok          bool    `json:"ok"`
	Summary     string  `json:"summary"`
	StartsAt    string  `json:"starts_at"` // human-readable, Nepal time
	StartsAtISO string  `json:"starts_at_iso"`
	Critical    bool    `json:"critical"`
	Recurring   bool    `json:"recurring"`
	TimeOfDay   string  `json:"time_of_day,omitempty"`
	Amount      float64 `json:"amount"`
	Unit        string  `json:"unit"`
	Freq        string  `json:"freq,omitempty"`
	Interval    int     `json:"interval,omitempty"`
	EventID     string  `json:"event_id,omitempty"`
	Created     bool    `json:"created"`
}

// Run executes the pipeline. Set now to time.Time{} to use the current moment.
func Run(ctx context.Context, req *Request, svc *gcal.Service, now time.Time) (*Response, error) {
	if req == nil || req.Text == "" {
		return nil, apperr.BadRequest("text is required")
	}

	t := time.Now()
	intent, err := nlu.Parse(ctx, req.Text)
	timing.Since("nlu", t)
	if err != nil {
		return nil, apperr.Unprocessable(err.Error())
	}

	t = time.Now()
	start, err := gcal.ComputeStartTime(intent, now)
	timing.Since("compute_time", t)
	if err != nil {
		return nil, apperr.Unprocessable(err.Error())
	}

	resp := &Response{
		Summary:     intent.Summary,
		StartsAt:    start.In(timeparse.AppTZ()).Format("2006-01-02 3:04 PM"),
		StartsAtISO: start.Format(time.RFC3339),
		Critical:    intent.Critical,
		Recurring:   intent.Recurring,
		TimeOfDay:   intent.TimeOfDay,
		Amount:      intent.Amount,
		Unit:        intent.Unit,
		Freq:        intent.Freq,
		Interval:    intent.Interval,
	}

	if req.Create {
		if svc == nil {
			return nil, apperr.Internal(errors.New("calendar service not configured"))
		}
		event := gcal.BuildEvent(intent, start)
		t = time.Now()
		id, err := svc.CreateEvent(ctx, event)
		timing.Since("gcal_create", t)
		if err != nil {
			return nil, apperr.From(fmt.Errorf("create reminder: %w", err))
		}
		resp.EventID = id
		resp.Created = true
	}

	resp.Ok = true
	return resp, nil
}
