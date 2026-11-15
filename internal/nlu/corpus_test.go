package nlu

import (
	"context"
	"testing"
)

// corpusCase is one real-world phrase and what the fallback parser must
// produce for it. Add a line here whenever a phrase is parsed wrongly.
type corpusCase struct {
	in      string
	summary string
	amount  float64
	unit    string
	tod     string
	recur   bool
	freq    string
}

var corpus = []corpusCase{
	// Plain durations.
	{in: "remind me in 45 days to pay bills", summary: "pay bills", amount: 45, unit: "D"},
	{in: "remind to take medicine in 30 minutes", summary: "take medicine", amount: 30, unit: "MIN"},
	{in: "finish report in 2 hours", summary: "finish report", amount: 2, unit: "H"},

	// Spelled-out numbers.
	{in: "remind me in three days to renew passport", summary: "renew passport", amount: 3, unit: "D"},
	{in: "remind me in twelve hours to call back", summary: "call back", amount: 12, unit: "H"},
	{in: "remind me in two weeks to submit timesheet", summary: "submit timesheet", amount: 14, unit: "D"},

	// Articles as quantity.
	{in: "remind me in a week to call the bank", summary: "call the bank", amount: 7, unit: "D"},
	{in: "remind me in an hour to stretch", summary: "stretch", amount: 1, unit: "H"},
	{in: "remind me in half an hour to take tea", summary: "take tea", amount: 30, unit: "MIN"},

	// Compound durations.
	{in: "remind me in 2 hours 30 minutes to leave", summary: "leave", amount: 150, unit: "MIN"},
	{in: "remind me in 1 hour and 15 minutes to call mom", summary: "call mom", amount: 75, unit: "MIN"},

	// Relative day words.
	{in: "remind me day after tomorrow to pay rent", summary: "pay rent", amount: 2, unit: "D"},
	{in: "remind me tomorrow to pay rent", summary: "pay rent", amount: 1, unit: "D"},
	{in: "remind me next week to pay rent", summary: "pay rent", amount: 7, unit: "D"},
	{in: "remind me next month to pay rent", summary: "pay rent", amount: 1, unit: "M"},

	// Recurrence.
	{in: "remind me weekly to water plants", summary: "water plants", recur: true, freq: "WEEKLY"},
	{in: "remind me every 2 weeks to clean the filter", summary: "clean the filter", amount: 14, unit: "D", recur: true, freq: "WEEKLY"},
	{in: "remind me every day at 8pm to take vitamins", summary: "take vitamins", tod: "8pm", recur: true, freq: "DAILY"},
	{in: "renew gym membership every 2 months", summary: "renew gym membership", amount: 2, unit: "M", recur: true, freq: "MONTHLY"},

	// Time of day.
	{in: "remind me tomorrow at 5:30pm to call mom", summary: "call mom", amount: 1, unit: "D", tod: "5:30pm"},
	{in: "remind me to call mom at 2am", summary: "call mom", tod: "2am"},
}

func TestCorpus(t *testing.T) {
	ctx := context.Background()
	for _, c := range corpus {
		got, err := ParseFallback(ctx, c.in)
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
		if got.Recurring != c.recur || (c.freq != "" && got.Freq != c.freq) {
			t.Errorf("%q: recurrence = (recur:%v,freq:%q), want (recur:%v,freq:%q)",
				c.in, got.Recurring, got.Freq, c.recur, c.freq)
		}
	}
}
