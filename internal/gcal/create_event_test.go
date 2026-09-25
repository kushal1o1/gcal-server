package gcal

import (
	"testing"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/nlu"
	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

func TestComputeStartTime(t *testing.T) {
	now := time.Date(2026, 9, 25, 21, 15, 0, 0, timeparse.NepalTZ)

	cases := []struct {
		name   string
		intent *nlu.EventIntent
		want   time.Time
	}{
		{"sub-day minutes keep exact clock", &nlu.EventIntent{Summary: "x", Amount: 30, Unit: "MIN"},
			now.Add(30 * time.Minute)},
		{"sub-day hours keep exact clock", &nlu.EventIntent{Summary: "x", Amount: 2, Unit: "H"},
			now.Add(2 * time.Hour)},
		{"day offset defaults to 9am", &nlu.EventIntent{Summary: "x", Amount: 45, Unit: "D"},
			time.Date(2026, 11, 9, 9, 0, 0, 0, timeparse.NepalTZ)},
		{"offset plus clock applies clock", &nlu.EventIntent{Summary: "x", Amount: 1, Unit: "D", TimeOfDay: "9:30am"},
			time.Date(2026, 9, 26, 9, 30, 0, 0, timeparse.NepalTZ)},
		{"passed time-of-day rolls to tomorrow", &nlu.EventIntent{Summary: "x", TimeOfDay: "5:30pm"},
			time.Date(2026, 9, 26, 17, 30, 0, 0, timeparse.NepalTZ)},
	}
	for _, c := range cases {
		got, err := ComputeStartTime(c.intent, now)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildEvent(t *testing.T) {
	start := time.Date(2026, 9, 25, 9, 0, 0, 0, timeparse.NepalTZ)

	monthly := &nlu.EventIntent{Summary: "pay rent", Recurring: true, Freq: "MONTHLY", Interval: 1}
	if e := BuildEvent(monthly, start); e.Recurrence[0] != "RRULE:FREQ=MONTHLY;INTERVAL=1" {
		t.Errorf("monthly RRULE = %q", e.Recurrence[0])
	}

	biweekly := &nlu.EventIntent{Summary: "clean filter", Recurring: true, Freq: "WEEKLY", Interval: 2}
	if e := BuildEvent(biweekly, start); e.Recurrence[0] != "RRULE:FREQ=WEEKLY;INTERVAL=2" {
		t.Errorf("biweekly RRULE = %q", e.Recurrence[0])
	}

	unitDriven := &nlu.EventIntent{Summary: "x", Recurring: true, Amount: 14, Unit: "D"}
	if e := BuildEvent(unitDriven, start); e.Recurrence[0] != "RRULE:FREQ=DAILY;INTERVAL=14" {
		t.Errorf("unit RRULE = %q", e.Recurrence[0])
	}

	bareRecur := &nlu.EventIntent{Summary: "x", Recurring: true}
	if e := BuildEvent(bareRecur, start); e.Recurrence[0] != "RRULE:FREQ=DAILY;INTERVAL=1" {
		t.Errorf("bare recur RRULE = %q", e.Recurrence[0])
	}

	normal := BuildEvent(&nlu.EventIntent{Summary: "x"}, start)
	if len(normal.Reminders.Overrides) != 1 || normal.Reminders.Overrides[0].Method != "popup" || normal.Reminders.Overrides[0].Minutes != 0 {
		t.Errorf("normal reminders = %+v", normal.Reminders.Overrides)
	}

	critical := BuildEvent(&nlu.EventIntent{Summary: "x", Critical: true}, start)
	want := []int{24 * 60, 60, 0, 0}
	if len(critical.Reminders.Overrides) != 4 {
		t.Fatalf("critical overrides = %d, want 4", len(critical.Reminders.Overrides))
	}
	for i, r := range critical.Reminders.Overrides {
		if int(r.Minutes) != want[i] {
			t.Errorf("critical override %d = %d minutes, want %d", i, r.Minutes, want[i])
		}
	}

	if end := start.Add(time.Hour); BuildEvent(&nlu.EventIntent{Summary: "x"}, start).End.DateTime != end.Format(time.RFC3339) {
		t.Error("event duration is not 1 hour")
	}
}