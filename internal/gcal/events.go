package gcal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

// ErrNotFound is returned by DeleteEvent when Google reports the event is gone.
var ErrNotFound = errors.New("event not found")

const (
	pageSize = 100
	// fetchUpcomingPageSize is the page size for the expanded upcoming instance
	// listing: larger pages surface the distinct one-off events in fewer
	// round-trips (each page costs hundreds of ms on Google's side).
	fetchUpcomingPageSize = 250
	maxPages              = 10
	defaultListLimit      = 5
	// fetchMastersMaxResults is the page size for the recurring-master sweep:
	// Google caps events.list at 2500, so one round-trip covers most calendars
	// instead of the default-100 sweep the instance listing uses.
	fetchMastersMaxResults = 2500
	// MaxListLimit caps the largest limit a caller may request with listEvents.
	MaxListLimit = 500
)

// ReminderSetting mirrors a single Google reminder override: method is
// "email", "popup" or "notification"; minutes is how many minutes before the
// event it fires (0 = at event time).
type ReminderSetting struct {
	Method  string
	Minutes int
}

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
	Reminders         []ReminderSetting
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
//     orderBy=startTime) with recurring series deduped to their first
//     upcoming occurrence so each series appears once, then each kept
//     occurrence is enriched from its MASTER event so priority, recurrence
//     label and reminders reflect the whole series (Google omits these on
//     expanded instances);
//   - RecurringOnly: master events only (singleEvents=false), each series
//     shown once, past included.
//
// It returns the events plus hasMore, reporting whether more events exist
// beyond those returned. To honour large limits the underlying Google pages
// are fetched until `limit` distinct (deduped+filtered) items are available
// or the result set is exhausted (up to maxPages pages).
func (s *Service) ListEvents(ctx context.Context, opts ListOptions) ([]EventItem, bool, error) {
	if opts.RecurringOnly {
		return s.listMasters(ctx, opts)
	}
	return s.listUpcoming(ctx, opts)
}

func (s *Service) listUpcoming(ctx context.Context, opts ListOptions) ([]EventItem, bool, error) {
	limit := opts.Limit
	if limit < 0 {
		limit = defaultListLimit
	}

	var raw []*calendar.Event
	var masters map[string]*calendar.Event
	now := time.Now().In(timeparse.AppTZ()).Format(time.RFC3339)
	nowT := time.Now()
	next := ""
	count := 0
	for page := 0; page < maxPages; page++ {
		start := time.Now()
		req := s.Svc.Events.List(s.CalendarID).
			TimeMin(now).
			MaxResults(fetchUpcomingPageSize).
			SingleEvents(true).
			OrderBy("startTime").
			Context(ctx)
		if next != "" {
			req.PageToken(next)
		}
		resp, err := req.Do()
		if err != nil {
			return nil, false, fmt.Errorf("fetch events: %w", err)
		}
		log.Printf("[events] instance page %d: %d items, %.0fms", page, len(resp.Items), float64(time.Since(start))*1e-3)
		resp.Items = dropFinished(resp.Items, nowT)
		raw = append(raw, resp.Items...)
		next = resp.NextPageToken
		if limit == 0 {
			return []EventItem{}, next != "" || len(raw) > 0, nil
		}
		prevCount := count
		if len(raw) > 0 {
			count = s.previewCount(ctx, raw, opts, &masters)
		}
		// A page that added no *new distinct* events means everything relevant is
		// already in hand. Without this, an infinite recurring series (one
		// instance per Google page, collapsed by dedupeRecurring) would sweep
		// every page of the calendar.
		if page > 0 && count == prevCount {
			next = ""
			break
		}
		if next == "" || count >= limit {
			break
		}
	}

	// has_more only when the requested limit was actually filled, so the
	// "load more" affordance disappears once everything distinct is shown
	// (deduping collapses infinite recurring instances into one series).
	hasMore := next != "" && count >= limit

	events := make([]EventItem, 0, len(raw))
	for _, e := range raw {
		events = append(events, eventFromCalendar(e))
	}
	events = dedupeRecurring(events)
	if masters != nil {
		enrichRecurring(events, masters)
	}
	events = filterItems(events, opts)
	if limit > 0 && limit < len(events) {
		events = events[:limit]
	}
	return events, hasMore, nil
}

func (s *Service) listMasters(ctx context.Context, opts ListOptions) ([]EventItem, bool, error) {
	limit := opts.Limit // <0 = every recurring series (CLI -A)
	next := ""
	var items []EventItem
	for page := 0; page < maxPages; page++ {
		req := s.Svc.Events.List(s.CalendarID).
			MaxResults(pageSize).
			SingleEvents(false).
			OrderBy("updated").
			Context(ctx)
		if next != "" {
			req.PageToken(next)
		}
		resp, err := req.Do()
		if err != nil {
			return nil, false, fmt.Errorf("fetch master recurring events: %w", err)
		}
		next = resp.NextPageToken
		for _, e := range resp.Items {
			if len(e.Recurrence) > 0 {
				items = append(items, eventFromCalendar(e))
			}
		}
		if limit == 0 {
			return []EventItem{}, next != "" || len(items) > 0, nil
		}
		if next == "" || (limit > 0 && len(items) >= limit) {
			break
		}
	}
	// has_more only when masters fill the requested limit (default = all).
	hasMore := next != "" && limit > 0 && len(items) >= limit
	items = filterItems(items, opts)
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// previewCount converts, dedupes and filters the raw instances gathered so
// far (enriching from masters when any are recurring) and returns how many
// distinct events would be returned. Used to stop paging once a limit is met.
func (s *Service) previewCount(ctx context.Context, raw []*calendar.Event, opts ListOptions, masters *map[string]*calendar.Event) int {
	if hasRecurringItem(raw) {
		s.ensureMasters(ctx, masters)
	}
	events := make([]EventItem, 0, len(raw))
	for _, e := range raw {
		events = append(events, eventFromCalendar(e))
	}
	events = dedupeRecurring(events)
	if *masters != nil {
		enrichRecurring(events, *masters)
	}
	return len(filterItems(events, opts))
}

// ensureMasters lazily loads every recurring MASTER event into *m, degrading
// to nil on error so listing still works without enrichment.
func (s *Service) ensureMasters(ctx context.Context, m *map[string]*calendar.Event) {
	if *m != nil {
		return
	}
	*m, _ = s.fetchMastersMap(ctx)
}

// fetchMastersMap returns every recurring master keyed by id.
func (s *Service) fetchMastersMap(ctx context.Context) (map[string]*calendar.Event, error) {
	out := map[string]*calendar.Event{}
	next := ""
	start := time.Now()
	for page := 0; page < maxPages; page++ {
		req := s.Svc.Events.List(s.CalendarID).
			MaxResults(fetchMastersMaxResults).
			SingleEvents(false).
			OrderBy("updated").
			Context(ctx)
		if next != "" {
			req.PageToken(next)
		}
		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("fetch master recurring events: %w", err)
		}
		log.Printf("[events] masters page %d: %d items, %.0fms", page, len(resp.Items), float64(time.Since(start))*1e-3)
		for _, e := range resp.Items {
			if len(e.Recurrence) > 0 {
				out[e.Id] = e
			}
		}
		next = resp.NextPageToken
		if next == "" {
			break
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

// dropFinished removes expanded instances whose end has already passed so
// finished reminders stop surfacing as "upcoming" (Google's own timeMin
// filter keeps single events that merely overlap "now", and past events are
// never removed from the calendar).
func dropFinished(items []*calendar.Event, now time.Time) []*calendar.Event {
	out := items[:0]
	for _, e := range items {
		if e.End != nil && e.End.DateTime != "" {
			end, err := time.Parse(time.RFC3339, e.End.DateTime)
			if err == nil && end.Before(now) {
				continue
			}
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
		Reminders: remindersFrom(e),
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

// remindersFrom flattens an event's reminder overrides; nil when none are set.
func remindersFrom(e *calendar.Event) []ReminderSetting {
	if e.Reminders == nil || len(e.Reminders.Overrides) == 0 {
		return nil
	}
	out := make([]ReminderSetting, 0, len(e.Reminders.Overrides))
	for _, o := range e.Reminders.Overrides {
		if o == nil {
			continue
		}
		out = append(out, ReminderSetting{Method: o.Method, Minutes: int(o.Minutes)})
	}
	return out
}

// hasRecurringItem reports whether any raw event is part of a recurring series.
func hasRecurringItem(raw []*calendar.Event) bool {
	for _, e := range raw {
		if isRecurring(e) {
			return true
		}
	}
	return false
}

// enrichRecurring overwrites an instance's priority, recurrence label and
// reminders from its MASTER event — Google returns neither on expanded
// instances, which would otherwise mislabel recurring critical series.
func enrichRecurring(events []EventItem, masters map[string]*calendar.Event) {
	for i := range events {
		if !events[i].Recurring {
			continue
		}
		m, ok := masters[events[i].MasterID]
		if !ok {
			continue
		}
		events[i].Critical = isCritical(m)
		events[i].Reminders = remindersFrom(m)
		if len(m.Recurrence) > 0 {
			events[i].RecurrenceDisplay = ParseRecurrenceDisplay(m.Recurrence)
		}
	}
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
		return t.In(timeparse.AppTZ()), ""
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

// ReminderPatch describes the new reminder config for an event. UseDefault
// restores the calendar default (clears overrides); otherwise Reminders are
// applied verbatim.
type ReminderPatch struct {
	UseDefault bool
	Reminders  []ReminderSetting
}

// EditReminders replaces an event's reminder config. Id may be an instance id
// (a single occurrence) or a master id (the whole series, which cascades to
// every instance). A GET-then-UPDATE is used because Google's PATCH only
// merges nested fields, so the old overrides would survive alongside the new.
func (s *Service) EditReminders(ctx context.Context, id string, patch ReminderPatch) error {
	ev, err := s.Svc.Events.Get(s.CalendarID, id).Context(ctx).Do()
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && (gerr.Code == 404 || gerr.Code == 410) {
			return ErrNotFound
		}
		return fmt.Errorf("get calendar event for edit: %w", err)
	}
	if patch.UseDefault {
		ev.Reminders = &calendar.EventReminders{UseDefault: true}
	} else {
		// Google's API treats a missing useDefault as true, so the false must
		// be force-serialized whenever overrides are supplied.
		ev.Reminders = &calendar.EventReminders{
			UseDefault:      false,
			ForceSendFields: []string{"UseDefault"},
		}
		for _, r := range patch.Reminders {
			// minutes:0 must be serialized even though the generated field is
			// omitempty, or Google rejects the override as missing minutes.
			ev.Reminders.Overrides = append(ev.Reminders.Overrides, &calendar.EventReminder{
				Method:          r.Method,
				Minutes:         int64(r.Minutes),
				ForceSendFields: []string{"Minutes"},
			})
		}
	}
	if _, err := s.Svc.Events.Update(s.CalendarID, id, ev).Context(ctx).Do(); err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && (gerr.Code == 404 || gerr.Code == 410) {
			return ErrNotFound
		}
		return fmt.Errorf("edit calendar event reminders: %w", err)
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
	masters, err := s.fetchMastersMap(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]deleteTarget, 0, len(masters))
	for _, m := range masters {
		targets = append(targets, deleteTarget{id: m.Id, summary: summaryOr(m), series: true})
	}
	return s.bulkDelete(ctx, targets), nil
}

// fetchUpcomingAll gathers upcoming instances across pages (up to maxPages).
func (s *Service) fetchUpcomingAll(ctx context.Context) ([]*calendar.Event, error) {
	var out []*calendar.Event
	now := time.Now().In(timeparse.AppTZ()).Format(time.RFC3339)
	next := ""
	for page := 0; page < maxPages; page++ {
		req := s.Svc.Events.List(s.CalendarID).
			TimeMin(now).
			MaxResults(pageSize).
			SingleEvents(true).
			OrderBy("startTime").
			Context(ctx)
		if next != "" {
			req.PageToken(next)
		}
		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("fetch events: %w", err)
		}
		out = append(out, resp.Items...)
		next = resp.NextPageToken
		if next == "" {
			break
		}
	}
	return out, nil
}

// BulkDeleteCritical mirrors `gcal delete -C`: deletes every critical event.
// Recurring series are deleted once (master id) rather than once per upcoming
// instance, so a series never 404s on its own earlier deletion.
func (s *Service) BulkDeleteCritical(ctx context.Context) ([]BulkDeleteItem, error) {
	upcoming, err := s.fetchUpcomingAll(ctx)
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
			// Per-item errors go back to the client, so keep Google's text out
			// of the response; log it with the event id instead.
			log.Printf("bulk delete failed id=%s: %v", t.id, err)
			item.Err = "could not delete this event"
		} else {
			item.OK = true
		}
		out = append(out, item)
	}
	return out
}
