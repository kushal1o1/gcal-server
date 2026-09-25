package gcal

import (
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

func TestIsCritical(t *testing.T) {
	normal := &calendar.Event{}
	normal.Reminders = &calendar.EventReminders{}
	normal.Reminders.Overrides = []*calendar.EventReminder{{Method: "popup", Minutes: 0}}
	if isCritical(normal) {
		t.Error("1 override should not be critical")
	}

	critical := &calendar.Event{}
	critical.Reminders = &calendar.EventReminders{}
	critical.Reminders.Overrides = []*calendar.EventReminder{{}, {}, {}, {}}
	if !isCritical(critical) {
		t.Error("4 overrides should be critical")
	}

	if isCritical(&calendar.Event{}) {
		t.Error("nil reminders should not be critical")
	}
}

func TestIsRecurringAndMasterID(t *testing.T) {
	instance := &calendar.Event{Id: "inst1", RecurringEventId: "master1"}
	if !isRecurring(instance) {
		t.Error("recurringEventId should mark recurring")
	}
	if got := masterEventID(instance); got != "master1" {
		t.Errorf("master id = %q, want master1", got)
	}

	master := &calendar.Event{Id: "master1", Recurrence: []string{"RRULE:FREQ=WEEKLY;INTERVAL=1"}}
	if !isRecurring(master) {
		t.Error("recurrence should mark recurring")
	}
	if got := masterEventID(master); got != "master1" {
		t.Errorf("master id = %q, want master1", got)
	}

	plain := &calendar.Event{Id: "plain1"}
	if isRecurring(plain) {
		t.Error("plain event should not be recurring")
	}
}

func TestParseRecurrenceDisplay(t *testing.T) {
	cases := []struct {
		rules []string
		want  string
	}{
		{nil, "Unknown"},
		{[]string{"RRULE:FREQ=DAILY;INTERVAL=1"}, "Every 1 day"},
		{[]string{"RRULE:FREQ=DAILY;INTERVAL=14"}, "Every 14 days"},
		{[]string{"RRULE:FREQ=DAILY"}, "Daily"},
		{[]string{"RRULE:FREQ=WEEKLY;INTERVAL=1"}, "Weekly"},
		{[]string{"RRULE:FREQ=MONTHLY;INTERVAL=2"}, "Every 2 months"},
		{[]string{"RRULE:FREQ=MONTHLY"}, "Monthly"},
		{[]string{"RRULE:FREQ=YEARLY;INTERVAL=1"}, "Yearly"},
		{[]string{"RRULE:FREQ=HOURLY;INTERVAL=2"}, "Hourly"},
		{[]string{"RRULE:FREQ=FREQ=YEARSTRONG"}, "Custom"},
	}
	for _, c := range cases {
		if got := ParseRecurrenceDisplay(c.rules); got != c.want {
			t.Errorf("ParseRecurrenceDisplay(%q) = %q, want %q", c.rules, got, c.want)
		}
	}
}

func TestEventFromCalendar(t *testing.T) {
	critical := &calendar.Event{Id: "c1", Summary: "pay rent"}
	critical.Reminders = &calendar.EventReminders{}
	critical.Reminders.Overrides = []*calendar.EventReminder{{}, {}, {}, {}}
	critical.Start = &calendar.EventDateTime{DateTime: "2026-09-26T09:00:00+05:45"}
	item := eventFromCalendar(critical)
	if !item.Critical || item.Recurring {
		t.Errorf("critical flag = %v, recurring = %v", item.Critical, item.Recurring)
	}
	if !item.Start.Equal(time.Date(2026, 9, 26, 9, 0, 0, 0, timeparse.NepalTZ)) {
		t.Errorf("start = %v", item.Start)
	}

	recurring := &calendar.Event{
		Id:               "m1",
		Recurrence:       []string{"RRULE:FREQ=MONTHLY;INTERVAL=1"},
		RecurringEventId: "m1",
	}
	recurring.Start = &calendar.EventDateTime{Date: "2026-09-26"}
	item = eventFromCalendar(recurring)
	if !item.Recurring {
		t.Error("recurring master not detected")
	}
	if item.RecurrenceDisplay != "Every 1 month" {
		t.Errorf("recurrence display = %q", item.RecurrenceDisplay)
	}
	if item.StartDate != "2026-09-26" {
		t.Errorf("start date = %q", item.StartDate)
	}

	empty := eventFromCalendar(&calendar.Event{})
	if empty.Summary != "No title" {
		t.Errorf("default summary = %q", empty.Summary)
	}
}

func TestDedupeRecurring(t *testing.T) {
	events := []EventItem{
		{ID: "inst1", MasterID: "m1", Recurring: true},
		{ID: "inst2", MasterID: "m1", Recurring: true},
		{ID: "inst3", MasterID: "m2", Recurring: true},
		{ID: "p1", MasterID: "p1", Recurring: false},
		{ID: "p2", MasterID: "p2", Recurring: false},
	}
	got := dedupeRecurring(events)
	if len(got) != 4 {
		t.Fatalf("dedupe returned %d events, want 4", len(got))
	}
	counts := map[string]int{}
	for _, e := range got {
		counts[e.MasterID]++
	}
	if counts["m1"] != 1 || counts["m2"] != 1 {
		t.Errorf("series masters repeated: %v", counts)
	}
	if got[0].ID != "inst1" {
		t.Errorf("kept %q, want inst1 (earliest occurrence)", got[0].ID)
	}
}

func TestFilterItems(t *testing.T) {
	all := []EventItem{
		{ID: "n1", Summary: "normal"},
		{ID: "c1", Summary: "critical", Critical: true},
	}

	if got := filterItems(all, ListOptions{CriticalOnly: true}); len(got) != 1 || got[0].ID != "c1" {
		t.Errorf("critical_only = %d events", len(got))
	}
	if got := filterItems(all, ListOptions{NormalOnly: true}); len(got) != 1 || got[0].ID != "n1" {
		t.Errorf("normal_only = %d events", len(got))
	}
	if got := filterItems(all, ListOptions{}); len(got) != 2 {
		t.Errorf("no filter = %d events", len(got))
	}
}

func TestRemindersFrom(t *testing.T) {
	if got := remindersFrom(&calendar.Event{}); got != nil {
		t.Errorf("nil reminders = %v, want nil", got)
	}
	useDefault := &calendar.Event{Reminders: &calendar.EventReminders{UseDefault: true}}
	if got := remindersFrom(useDefault); got != nil {
		t.Errorf("default reminders = %v, want nil", got)
	}
	normal := &calendar.Event{Reminders: &calendar.EventReminders{
		Overrides: []*calendar.EventReminder{{Method: "popup", Minutes: 0}},
	}}
	got := remindersFrom(normal)
	if len(got) != 1 || got[0].Method != "popup" || got[0].Minutes != 0 {
		t.Errorf("normal reminders = %v", got)
	}
	critical := &calendar.Event{Reminders: &calendar.EventReminders{
		Overrides: []*calendar.EventReminder{
			{Method: "email", Minutes: 1440},
			{Method: "popup", Minutes: 60},
			{Method: "email", Minutes: 0},
			{Method: "popup", Minutes: 0},
		},
	}}
	got = remindersFrom(critical)
	if len(got) != 4 || got[0].Minutes != 1440 {
		t.Errorf("critical reminders = %v", got)
	}
}

func TestEnrichRecurring(t *testing.T) {
	master := &calendar.Event{
		Id:         "m1",
		Recurrence: []string{"RRULE:FREQ=MONTHLY;INTERVAL=1"},
		Reminders: &calendar.EventReminders{
			Overrides: []*calendar.EventReminder{{Method: "email", Minutes: 1440}, {Method: "popup", Minutes: 60}},
		},
	}
	events := []EventItem{
		{ID: "inst1", MasterID: "m1", Summary: "ocean eyes", Recurring: true},
		{ID: "p1", MasterID: "p1", Summary: "plain", Recurring: false},
	}
	masters := map[string]*calendar.Event{"m1": master}
	enrichRecurring(events, masters)

	if !events[0].Critical {
		t.Error("recurring instance should inherit critical from master")
	}
	if events[0].RecurrenceDisplay != "Every 1 month" {
		t.Errorf("recurrence display = %q", events[0].RecurrenceDisplay)
	}
	if len(events[0].Reminders) != 2 || events[0].Reminders[0].Method != "email" {
		t.Errorf("instance reminders = %v", events[0].Reminders)
	}
	if events[1].Critical || events[1].Recurring || len(events[1].Reminders) != 0 {
		t.Error("plain event should be untouched by enrichment")
	}
}
