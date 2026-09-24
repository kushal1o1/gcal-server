// Package nlu converts natural-language reminder phrases into a structured
// EventIntent that gcal-svr can schedule.
package nlu

import (
	"fmt"
	"strings"
)

// EventIntent is the structured result of NLU. It maps 1:1 onto gcal's
// command grammar: remind <time> <event> [-A], plus the critical flag.
type EventIntent struct {
	Summary   string  `json:"summary"`              // event description/title
	Amount    float64 `json:"amount"`               // time offset (matches ParseTimeUnit)
	Unit      string  `json:"unit"`                 // D M Y H MIN ("" => today)
	TimeOfDay string  `json:"time_of_day,omitempty"` // "5:30pm", "14:30"; parsed by ParseTime
	Critical  bool    `json:"critical"`             // CLI's capital-R "Remind"
	Recurring bool    `json:"recurring"`            // CLI's -A flag
}

// Usable reports whether the intent is enough to schedule an event.
func (e *EventIntent) Usable() error {
	summary := strings.TrimSpace(e.Summary)
	if summary == "" {
		return fmt.Errorf("no event summary extracted")
	}
	e.Summary = summary
	return nil
}