package gcal

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/kushal1o1/gcal-svr/internal/nlu"
	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

// ComputeStartTime turns an EventIntent into an actual datetime, mirroring
// gcal's create_calendar_event: relative offsets are applied on top of "now",
// an explicit time-of-day overrides the clock, and otherwise events default to
// 9:00 AM on the target day.
//
// When only a time-of-day was given and it already passed today, it rolls to
// the next day so reminders never land in the past.
func ComputeStartTime(intent *nlu.EventIntent, now time.Time) (time.Time, error) {
	if now.IsZero() {
		now = time.Now().In(timeparse.AppTZ())
	}

	var eventDT time.Time
	if intent.Amount > 0 {
		unit := intent.Unit
		if unit == "" {
			unit = "D"
		}
		var err error
		eventDT, err = timeparse.CalculateFutureDate(intent.Amount, unit, now)
		if err != nil {
			return time.Time{}, err
		}
	} else {
		eventDT = now
	}

	if intent.TimeOfDay != "" {
		hour, minute, err := timeparse.ParseTime(intent.TimeOfDay)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad time_of_day %q: %w", intent.TimeOfDay, err)
		}
		eventDT = time.Date(eventDT.Year(), eventDT.Month(), eventDT.Day(), hour, minute, 0, 0, eventDT.Location())
		if intent.Amount <= 0 && eventDT.Before(now) {
			eventDT = eventDT.AddDate(0, 0, 1)
		}
	} else if intent.Unit == "H" || intent.Unit == "MIN" {
		// Relative sub-day offsets keep their exact clock (now + offset),
		// matching the CLI so "in 30 minutes" never lands in the past.
	} else {
		eventDT = time.Date(eventDT.Year(), eventDT.Month(), eventDT.Day(), 9, 0, 0, 0, eventDT.Location())
	}

	return eventDT, nil
}

// BuildEvent constructs the calendar.Event payload the same way the CLI does:
// 1h duration, popup reminder at event time, extra email+popup overrides for
// critical events, and an RRULE when recurring.
func BuildEvent(intent *nlu.EventIntent, start time.Time) *calendar.Event {
	startISO := start.Format(time.RFC3339)
	endISO := start.Add(time.Hour).Format(time.RFC3339)
	tz := timeparse.AppTZ().String()

	event := &calendar.Event{
		Summary: intent.Summary,
		Start:   &calendar.EventDateTime{DateTime: startISO, TimeZone: tz},
		End:     &calendar.EventDateTime{DateTime: endISO, TimeZone: tz},
		Reminders: &calendar.EventReminders{
			// UseDefault must be force-sent as false: Go's omitempty drops it,
			// and Google's API default (true) conflicts with custom overrides.
			UseDefault:      false,
			ForceSendFields: []string{"UseDefault"},
			Overrides:       []*calendar.EventReminder{eventReminder("popup", 0)},
		},
	}

	if intent.Critical {
		event.Reminders.Overrides = []*calendar.EventReminder{
			eventReminder("email", 24*60), // 1 day before
			eventReminder("popup", 60),    // 1 hour before
			eventReminder("email", 0),     // at event time
			eventReminder("popup", 0),     // at event time
		}
	}

	if intent.Recurring {
		freq := intent.Freq
		if freq == "" {
			freq = nlu.UnitToFreq(intent.Unit)
		}
		if freq == "" {
			freq = "DAILY"
		}
		interval := intent.Interval
		if interval < 1 {
			interval = int(intent.Amount)
		}
		if interval < 1 {
			interval = 1
		}
		event.Recurrence = []string{fmt.Sprintf("RRULE:FREQ=%s;INTERVAL=%d", freq, interval)}
	}

	return event
}

// eventReminder force-sends minutes so that 0 (= at event time) survives Go's
// omitempty, matching the explicit values the Python CLI always sent.
func eventReminder(method string, minutes int) *calendar.EventReminder {
	return &calendar.EventReminder{
		Method:          method,
		Minutes:         int64(minutes),
		ForceSendFields: []string{"Minutes"},
	}
}

// CreateEvent inserts the event into the shared calendar and returns its id.
func (s *Service) CreateEvent(ctx context.Context, event *calendar.Event) (string, error) {
	created, err := s.Svc.Events.Insert(s.CalendarID, event).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create calendar event: %w", err)
	}
	return created.Id, nil
}
