package nlu

import (
	"context"
	"strings"
	"testing"
)

// plainCase is a phrase as a person would actually say it, and what the parser
// must make of it. Add a line here whenever a real phrase is parsed wrongly.
type plainCase struct {
	in       string
	summary  string
	amount   float64
	unit     string
	tod      string
	critical bool
	recur    bool
	freq     string
	byDay    string
}

var plainEnglish = []plainCase{
	// Timing: durations and words.
	{in: "remind me in 2 weeks to renew passport", summary: "renew passport", amount: 14, unit: "D"},
	{in: "remind me in 3 days to pay the electricity bill", summary: "pay the electricity bill", amount: 3, unit: "D"},
	{in: "remind me in 45 days to renew insurance", summary: "renew insurance", amount: 45, unit: "D"},
	{in: "remind me tomorrow at 9am to take meds", summary: "take meds", amount: 1, unit: "D", tod: "9am"},
	{in: "remind me in an hour to stretch", summary: "stretch", amount: 1, unit: "H"},

	// Repeats: how people say "again and again".
	{in: "remind me to pay rent every month", summary: "pay rent", recur: true, freq: "MONTHLY"},
	{in: "remind me each month to pay rent", summary: "pay rent", recur: true, freq: "MONTHLY"},
	{in: "always remind me to drink water every 2 hours", summary: "drink water", amount: 2, unit: "H", recur: true, freq: "HOURLY"},
	{in: "every 2 weeks remind me to change the filter", summary: "change the filter", amount: 14, unit: "D", recur: true, freq: "WEEKLY"},
	{in: "daily remind me to take vitamins at 8am", summary: "take vitamins", tod: "8am", recur: true, freq: "DAILY"},
	{in: "remind me every 3 days to change water filter", summary: "change water filter", amount: 3, unit: "D", recur: true, freq: "DAILY"},

	// Weekdays: a one-off day, and a weekly repeat.
	{in: "remind me on friday to submit the report", summary: "submit the report", byDay: "FR"},
	{in: "remind me next tuesday at 3pm to call the dentist", summary: "call the dentist", tod: "3pm", byDay: "TU"},
	{in: "remind me every monday to take out the bins", summary: "take out the bins", recur: true, freq: "WEEKLY", byDay: "MO"},
	{in: "remind me every sunday at 7pm to call mom", summary: "call mom", tod: "7pm", recur: true, freq: "WEEKLY", byDay: "SU"},

	// Importance: critical when the person says it matters.
	{in: "remind me to submit the tax form by tomorrow, it's urgent", summary: "submit the tax form", amount: 1, unit: "D", critical: true},
	{in: "remind me to pay the electricity bill in 3 days, it's important", summary: "pay the electricity bill", amount: 3, unit: "D", critical: true},
	{in: "remind me to call bank in 3 days its urgent", summary: "call bank", amount: 3, unit: "D", critical: true},
	{in: "remind me to call bank in 3 days, it's urgent", summary: "call bank", amount: 3, unit: "D", critical: true},
	{in: "remind me to call bank in 3 days its important", summary: "call bank", amount: 3, unit: "D", critical: true},
	{in: "this is important remind me to call the lawyer tomorrow", summary: "call the lawyer", amount: 1, unit: "D", critical: true},
	{in: "don't miss the flight check-in in 2 hours", summary: "flight check-in", amount: 2, unit: "H", critical: true},

	// Summary cleanup: no stray punctuation or dangling words.
	{in: "remind me to call mom at 5pm", summary: "call mom", tod: "5pm"},
	{in: "this is important, remind me to call the lawyer tomorrow", summary: "call the lawyer", amount: 1, unit: "D", critical: true},
	{in: "remind me in 2 weeks to renew passport", summary: "renew passport", amount: 14, unit: "D"},

	// Bare hours: 7-11 is morning, 12 is noon, 1-6 is afternoon.
	{in: "remind me to call mom at 5", summary: "call mom", tod: "5pm"},
	{in: "remind me to take pills at 8", summary: "take pills", tod: "8am"},
	{in: "remind me to take lunch at 12", summary: "take lunch", tod: "12pm"},

	// More timing words people use.
	{in: "remind me to call mom in 10 minutes", summary: "call mom", amount: 10, unit: "MIN"},
	{in: "remind me to take the bread out in 15 mins", summary: "take the bread out", amount: 15, unit: "MIN"},
	{in: "in 5 hours remind me to check the oven", summary: "check the oven", amount: 5, unit: "H"},
	{in: "remind me next week to submit timesheet", summary: "submit timesheet", amount: 7, unit: "D"},
	{in: "remind me next month to renew gym", summary: "renew gym", amount: 1, unit: "M"},
	{in: "remind me tonight to call grandma", summary: "call grandma", tod: "8pm"},
	{in: "remind me today at 6pm to water the plants", summary: "water the plants", tod: "6pm"},
	{in: "remind me to leave in half an hour", summary: "leave", amount: 30, unit: "MIN"},
	{in: "remind me to call bank for 3 days", summary: "call bank", amount: 3, unit: "D"},
	{in: "remind me tomorrow at 5 to call mom", summary: "call mom", amount: 1, unit: "D", tod: "5pm"},

	// Weekdays with "this", "every" and "each".
	{in: "remind me this friday to return the book", summary: "return the book", byDay: "FR"},
	{in: "remind me on wednesday at 4pm to call the vet", summary: "call the vet", tod: "4pm", byDay: "WE"},
	{in: "remind me every thursday at 6pm to call my brother", summary: "call my brother", tod: "6pm", recur: true, freq: "WEEKLY", byDay: "TH"},
	{in: "remind me each saturday to clean the car", summary: "clean the car", recur: true, freq: "WEEKLY", byDay: "SA"},

	// Repeats with units and words.
	{in: "remind me every day at 7am to take my pills", summary: "take my pills", tod: "7am", recur: true, freq: "DAILY"},
	{in: "remind me every week to send the report", summary: "send the report", recur: true, freq: "WEEKLY"},
	{in: "remind me to check the tires every 3 months", summary: "check the tires", amount: 3, unit: "M", recur: true, freq: "MONTHLY"},
	{in: "remind me to stretch every hour", summary: "stretch", recur: true, freq: "HOURLY"},
	{in: "remind me every 30 minutes to drink water", summary: "drink water", amount: 30, unit: "MIN", recur: true, freq: "MINUTELY"},

	// Importance words in different places.
	{in: "remind me to call the doctor, it's really important", summary: "call the doctor", critical: true},
	{in: "don't miss the dentist tomorrow at 10am", summary: "dentist", amount: 1, unit: "D", tod: "10am", critical: true},
	{in: "remind me to pay the card bill asap", summary: "pay the card bill", critical: true},
	{in: "remind me to file taxes, it's an emergency", summary: "file taxes", critical: true},
	{in: "remind me to go to school, it is critical", summary: "go to school", critical: true},

	// Bare hours: 7-11 morning, 12 noon, 1-6 afternoon.
	{in: "remind me to call the office at 10", summary: "call the office", tod: "10am"},
	{in: "remind me to take my medicine at 9", summary: "take my medicine", tod: "9am"},
	{in: "remind me to call dad at 3", summary: "call dad", tod: "3pm"},
	{in: "remind me to take the cake out at 11:30", summary: "take the cake out", tod: "11:30"},
	{in: "remind me to take the bread out at 2:15", summary: "take the bread out", tod: "2:15pm"},
}

func TestPlainEnglish(t *testing.T) {
	ctx := context.Background()
	for _, c := range plainEnglish {
		got, err := Parse(ctx, c.in)
		if err != nil {
			t.Errorf("%q: error: %v", c.in, err)
			continue
		}
		if got.Summary != c.summary {
			t.Errorf("%q: summary = %q, want %q", c.in, got.Summary, c.summary)
		}
		if got.Amount != c.amount || got.Unit != c.unit {
			t.Errorf("%q: time = (%v,%q), want (%v,%q)", c.in, got.Amount, got.Unit, c.amount, c.unit)
		}
		if got.TimeOfDay != c.tod {
			t.Errorf("%q: time_of_day = %q, want %q", c.in, got.TimeOfDay, c.tod)
		}
		if got.Critical != c.critical {
			t.Errorf("%q: critical = %v, want %v", c.in, got.Critical, c.critical)
		}
		if got.Recurring != c.recur || (c.freq != "" && got.Freq != c.freq) {
			t.Errorf("%q: recurrence = (%v,%q), want (%v,%q)", c.in, got.Recurring, got.Freq, c.recur, c.freq)
		}
		if got.ByDay != c.byDay {
			t.Errorf("%q: by_day = %q, want %q", c.in, got.ByDay, c.byDay)
		}
	}
}

// "every 45" has no unit. The parser must refuse rather than guess.
func TestUnitlessEveryIsRefused(t *testing.T) {
	_, err := Parse(context.Background(), "remind me to pay rent every 45")
	if err == nil {
		t.Fatal("expected an error for \"every 45\" with no unit")
	}
	if !strings.Contains(err.Error(), "needs a unit") {
		t.Fatalf("error should ask for a unit, got: %v", err)
	}
}
