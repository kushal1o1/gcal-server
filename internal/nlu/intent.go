// Package nlu converts natural-language reminder phrases into a structured
// EventIntent that gcal-svr can schedule.
package nlu

import (
	"fmt"
	"strings"

	"github.com/kushal1o1/gcal-svr/internal/timeparse"
)

// EventIntent is the structured result of NLU. It maps 1:1 onto gcal's
// command grammar: remind <time> <event> [-A], plus the critical flag.
type EventIntent struct {
	Summary   string  `json:"summary"`               // event description/title
	Amount    float64 `json:"amount"`                // time offset (matches ParseTimeUnit)
	Unit      string  `json:"unit"`                  // D M Y H MIN ("" => today)
	TimeOfDay string  `json:"time_of_day,omitempty"` // "5:30pm", "14:30"; parsed by ParseTime
	Critical  bool    `json:"critical"`              // CLI's capital-R "Remind"
	Recurring bool    `json:"recurring"`             // CLI's -A flag
	Freq      string  `json:"freq,omitempty"`        // DAILY WEEKLY MONTHLY YEARLY HOURLY MINUTELY ("" => none)
	Interval  int     `json:"interval,omitempty"`    // repeat every N (1 when N not stated)
	ByDay     string  `json:"by_day,omitempty"`      // weekday: MO TU WE TH FR SA SU ("" => none)
}

var unitToFreq = map[string]string{
	"D":   "DAILY",
	"M":   "MONTHLY",
	"Y":   "YEARLY",
	"H":   "HOURLY",
	"MIN": "MINUTELY",
}

// UnitToFreq maps a time unit onto its matching RRULE frequency.
func UnitToFreq(unit string) string { return unitToFreq[unit] }

// Usable reports whether the intent is enough to schedule an event.
func (e *EventIntent) Usable() error {
	summary := strings.TrimSpace(e.Summary)
	if summary == "" {
		return fmt.Errorf("no event summary extracted")
	}
	e.Summary = summary
	return nil
}

// Validate checks every field against its allowed value set so that an intent
// from the model can never carry random data into the calendar.
func (e *EventIntent) Validate() error {
	if strings.TrimSpace(e.Summary) == "" {
		return fmt.Errorf("summary is empty")
	}
	if e.Amount < 0 {
		return fmt.Errorf("negative amount %v", e.Amount)
	}
	switch e.Unit {
	case "", "D", "M", "Y", "H", "MIN":
	default:
		return fmt.Errorf("invalid unit %q (allowed: D, M, Y, H, MIN, or empty)", e.Unit)
	}
	if e.TimeOfDay != "" {
		if _, _, err := timeparse.ParseTime(e.TimeOfDay); err != nil {
			return fmt.Errorf("invalid time_of_day %q", e.TimeOfDay)
		}
	}
	switch e.ByDay {
	case "", "MO", "TU", "WE", "TH", "FR", "SA", "SU":
	default:
		return fmt.Errorf("invalid by_day %q", e.ByDay)
	}
	switch e.Freq {
	case "", "DAILY", "WEEKLY", "MONTHLY", "YEARLY", "HOURLY", "MINUTELY":
	default:
		return fmt.Errorf("invalid freq %q", e.Freq)
	}
	if e.Interval < 0 {
		return fmt.Errorf("negative interval %d", e.Interval)
	}
	return nil
}

// Normalize fills in missing recurrence values with deterministic defaults so
// BuildEvent never has to guess.
func (e *EventIntent) Normalize() {
	if !e.Recurring {
		return
	}
	if e.Interval < 1 {
		e.Interval = 1
	}
	if e.Freq == "" {
		e.Freq = UnitToFreq(e.Unit)
	}
	if e.Freq == "" {
		e.Freq = "DAILY"
	}
}
