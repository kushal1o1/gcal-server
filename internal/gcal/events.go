package gcal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

// ErrNotFound is returned by DeleteEvent when Google reports the event is gone.
var ErrNotFound = errors.New("event not found")

// EventItem is a flattened, API-friendly description of a calendar event,
// mirroring the fields the gcal CLI exposes for list/delete.
type EventItem struct {
	ID                string
	MasterID          string
	Summary           string
	Start             time.Time // parsed start, zero for date-only events
	StartDate         string    // all-day (date-only) events: "2006-01-02"
	Critical          bool
	Recurring         bool
	RecurrenceDisplay string
}

// ListOptions mirrors gcal list flags: a limit, "show only MASTER recurring",
// and the mutually exclusive critical/normal priority filters.
type ListOptions struct {
	// Limit < 0 means unset (defaults to 5, or all for RecurringOnly).
	Limit         int
	RecurringOnly bool
	CriticalOnly  bool
	NormalOnly    bool
}

// ListEvents replicates `gcal list`:
//
//   - default: upcoming instances (timeMin=now, singleEvents=true,
//     orderBy=startTime, maxResults=100) with recurring series deduped to
//     their first upcoming occurrence so each series appears once;
//   - RecurringOnly: master events only (singleEvents=false), each series
//     shown once, past included.
//
// Priority filters and limit mirror the CLI's filter_events.
func (s *Service) ListEvents(ctx context.Context, opts ListOptions) ([]EventItem, error) {
	var raw []*calendar.Event
	var err error
	if opts.RecurringOnly {
		raw, err = s.fetchMasters(ctx)
	} else {
		raw, err = s.fetchUpcoming(ctx)
	}
	if err != nil {
		return nil, err
	}

	events := make([]EventItem, 0, len(raw))
	for _, e := range raw {
		events = append(events, eventFromCalendar(e))
	}

	// Recurring series are deduped before filtering/limit so "recurring shown
	// once" holds and a limit counts distinct events.
	if !opts.RecurringOnly {
		events = dedupeRecurring(events)
	}
	events = filterItems(events, opts)

	limit := opts.Limit
	if limit < 0 {
		limit = len(events)
		if !opts.RecurringOnly {
			limit = 5
		}
	}
	if limit > 0 && limit < len(events) {
		events = events[:limit]
	}

	return events, nil
}

// fetchUpcoming expands recurring events into upcoming instances.
func (s *Service) fetchUpcoming(ctx context.Context) ([]*calendar.Event, error) {
	now := time.Now().In(timeparse.NepalTZ).Format(time.RFC3339)
	resp, err := s.Svc.Events.List(s.CalendarID).
		TimeMin(now).
		MaxResults(100).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("fetch events: %w", err)
	}
	return resp.Items, nil
}

// fetchMasters returns MASTER events (singleEvents=false), keeping only the
// ones with a recurrence rule — exactly like gcal's fetch_master_recurring_events.
func (s *Service) fetchMasters(ctx context.Context) ([]*calendar.Event, error) {
	resp, err := s.Svc.Events.List(s.CalendarID).
		MaxResults(100).
		SingleEvents(false).
		OrderBy("updated").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("fetch master recurring events: %w", err)
	}
	out := make([]*calendar.Event, 0, len(resp.Items))
	for _, e := range resp.Items {
		if len(e.Recurrence) > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// filterItems applies the critical/normal priority filters (mutually
// exclusive), mirroring the CLI's filter_events.
func filterItems(events []EventItem, opts ListOptions) []EventItem {
	if !opts.CriticalOnly && !opts.NormalOnly {
		return events
	}
	out := make([]EventItem, 0, len(events))
	for _, e := range events {
		if opts.CriticalOnly && !e.Critical {
			continue
		}
		if opts.NormalOnly && e.Critical {
			continue
		}
		out = append(out, e)
	}
	return out
}

// dedupeRecurring keeps each recurring series once (its earliest occurrence),
// satisfying "recurring events must be shown once" in the default list.
func dedupeRecurring(events []EventItem) []EventItem {
	seen := make(map[string]bool, len(events))
	out := make([]EventItem, 0, len(events))
	for _, e := range events {
		if e.Recurring {
			if seen[e.MasterID] {
				continue
			}
			seen[e.MasterID] = true
		}
		out = append(out, e)
	}
	return out
}

// eventFromCalendar flattens a raw calendar event, replicating the CLI's
// is_critical (overrides > 1) and recurring/master-id logic.
func eventFromCalendar(e *calendar.Event) EventItem {
	item := EventItem{
		ID:        e.Id,
		Summary:   e.Summary,
		Critical:  isCritical(e),
		Recurring: isRecurring(e),
	}
	if item.Summary == "" {
		item.Summary = "No title"
	}
	item.MasterID = masterEventID(e)
	item.Start, item.StartDate = eventStart(e)
	// Only master events carry a recurrence rule, so only they get a
	// recurrence display (instances resolve to the series via recurringEventId).
	if len(e.Recurrence) > 0 {
		item.RecurrenceDisplay = ParseRecurrenceDisplay(e.Recurrence)
	}
	return item
}

func isCritical(e *calendar.Event) bool {
	return e.Reminders != nil && len(e.Reminders.Overrides) > 1
}

func isRecurring(e *calendar.Event) bool {
	return e.RecurringEventId != "" || len(e.Recurrence) > 0
}

func masterEventID(e *calendar.Event) string {
	if e.RecurringEventId != "" {
		return e.RecurringEventId
	}
	return e.Id
}

// eventStart parses an event's start into (datetime, date-only string). Timed
// events land in the Nepal timezone; all-day events keep their "2006-01-02".
func eventStart(e *calendar.Event) (time.Time, string) {
	if e.Start == nil {
		return time.Time{}, ""
	}
	if dt := e.Start.DateTime; dt != "" {
		t, err := time.Parse(time.RFC3339, dt)
		if err != nil {
			return time.Time{}, ""
		}
		return t.In(timeparse.NepalTZ), ""
	}
	return time.Time{}, e.Start.Date
}

var reInterval = regexp.MustCompile(`INTERVAL=(\d+)`)

// ParseRecurrenceDisplay mirrors gcal's _parse_recurrence_rule: turns the
// first RRULE into a human "Every N days" / "Weekly" / ... description.
func ParseRecurrenceDisplay(rules []string) string {
	if len(rules) == 0 {
		return "Unknown"
	}
	r := rules[0]
	switch {
	case strings.Contains(r, "FREQ=DAILY"):
		if m := reInterval.FindStringSubmatch(r); m != nil {
			return "Every " + m[1] + " day" + plural(m[1])
		}
		return "Daily"
	case strings.Contains(r, "FREQ=WEEKLY"):
		return "Weekly"
	case strings.Contains(r, "FREQ=MONTHLY"):
		if m := reInterval.FindStringSubmatch(r); m != nil {
			return "Every " + m[1] + " month" + plural(m[1])
		}
		return "Monthly"
	case strings.Contains(r, "FREQ=YEARLY"):
		return "Yearly"
	case strings.Contains(r, "FREQ=HOURLY"):
		return "Hourly"
	default:
		return "Custom"
	}
}

func plural(n string) string {
	if n == "1" {
		return ""
	}
	return "s"
}

// DeleteEvent removes a single event (id) from the calendar. For recurring
// events the id may be the instance id (single occurrence) or the master id
// (whole series, what `gcal delete <id> -A` does).
func (s *Service) DeleteEvent(ctx context.Context, id string) error {
	err := s.Svc.Events.Delete(s.CalendarID, id).Context(ctx).Do()
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && (gerr.Code == 404 || gerr.Code == 410) {
			return ErrNotFound
		}
		return fmt.Errorf("delete calendar event: %w", err)
	}
	return nil
}

// BulkDeleteItem reports one deleted event for the bulk endpoints.
type BulkDeleteItem struct {
	Summary string
	Series  bool
	OK      bool
	Err     string
}

// BulkDeleteRecurring mirrors `gcal delete -A`: deletes every master
// recurring series.
func (s *Service) BulkDeleteRecurring(ctx context.Context) ([]BulkDeleteItem, error) {
	masters, err := s.fetchMasters(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]deleteTarget, 0, len(masters))
	for _, m := range masters {
		targets = append(targets, deleteTarget{id: m.Id, summary: summaryOr(m), series: true})
	}
	return s.bulkDelete(ctx, targets), nil
}

// BulkDeleteCritical mirrors `gcal delete -C`: deletes every critical event.
// Recurring series are deleted once (master id) rather than once per upcoming
// instance, so a series never 404s on its own earlier deletion.
func (s *Service) BulkDeleteCritical(ctx context.Context) ([]BulkDeleteItem, error) {
	upcoming, err := s.fetchUpcoming(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var targets []deleteTarget
	for _, e := range upcoming {
		if !isCritical(e) {
			continue
		}
		summary := summaryOr(e)
		if isRecurring(e) {
			mid := masterEventID(e)
			if seen[mid] {
				continue
			}
			seen[mid] = true
			targets = append(targets, deleteTarget{id: mid, summary: summary, series: true})
			continue
		}
		targets = append(targets, deleteTarget{id: e.Id, summary: summary, series: false})
	}
	return s.bulkDelete(ctx, targets), nil
}

func summaryOr(e *calendar.Event) string {
	if e.Summary != "" {
		return e.Summary
	}
	return "No title"
}

type deleteTarget struct {
	id      string
	summary string
	series  bool
}

func (s *Service) bulkDelete(ctx context.Context, targets []deleteTarget) []BulkDeleteItem {
	out := make([]BulkDeleteItem, 0, len(targets))
	for _, t := range targets {
		item := BulkDeleteItem{Summary: t.summary, Series: t.series}
		if err := s.DeleteEvent(ctx, t.id); err != nil {
			item.Err = err.Error()
		} else {
			item.OK = true
		}
		out = append(out, item)
	}
	return out
}
