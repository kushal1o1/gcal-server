package gcal

import (
	"testing"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/nlu"
	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

func TestNextWeekdayScheduling(t *testing.T) {
	loc := timeparse.AppTZ()
	wedMorning := time.Date(2026, 11, 18, 10, 0, 0, 0, loc) // a Wednesday
	friMorning := time.Date(2026, 11, 20, 10, 0, 0, 0, loc) // a Friday
	friEvening := time.Date(2026, 11, 20, 20, 0, 0, 0, loc)

	cases := []struct {
		name   string
		now    time.Time
		intent *nlu.EventIntent
		want   time.Time
	}{
		{"next friday, default 9am", wedMorning,
			&nlu.EventIntent{Summary: "x", ByDay: "FR"},
			time.Date(2026, 11, 20, 9, 0, 0, 0, loc)},
		{"today is friday but 9am passed, rolls to next week", friMorning,
			&nlu.EventIntent{Summary: "x", ByDay: "FR"},
			time.Date(2026, 11, 27, 9, 0, 0, 0, loc)},
		{"friday at 5:30pm from friday morning stays today", friMorning,
			&nlu.EventIntent{Summary: "x", ByDay: "FR", TimeOfDay: "5:30pm"},
			time.Date(2026, 11, 20, 17, 30, 0, 0, loc)},
		{"friday at 9am after 9am passed rolls over", friEvening,
			&nlu.EventIntent{Summary: "x", ByDay: "FR"},
			time.Date(2026, 11, 27, 9, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got, err := ComputeStartTime(c.intent, c.now)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestWeeklyRRuleCarriesByDay(t *testing.T) {
	intent := &nlu.EventIntent{Summary: "bins", ByDay: "MO", Recurring: true, Freq: "WEEKLY", Interval: 1}
	ev := BuildEvent(intent, time.Now())
	if len(ev.Recurrence) != 1 || ev.Recurrence[0] != "RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO" {
		t.Fatalf("recurrence = %v", ev.Recurrence)
	}
}
