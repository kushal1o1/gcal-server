package nlu

import (
	"context"
	"testing"
)

func TestParse(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in       string
		summary  string
		amount   float64
		unit     string
		tod      string
		critical bool
		recur    bool
		freq     string
		interval int
	}{
		{"remind to service my bike 45 days from now", "service my bike", 45, "D", "", false, false, "", 0},
		{"remind me every month to pay rent", "pay rent", 0, "", "", false, true, "MONTHLY", 1},
		{"critical important meeting tomorrow at 5:30pm", "meeting", 1, "D", "5:30pm", true, false, "", 0},
		{"Remind me to call mom at 2am", "call mom", 0, "", "2am", true, false, "", 0},
		{"remind to take medicine in 30 minutes", "take medicine", 30, "MIN", "", false, false, "", 0},
		{"bro remind me to buy milk", "buy milk", 0, "", "", false, false, "", 0},
		{"remind me of bike service next week", "bike service", 7, "D", "", false, false, "", 0},
		{"finish report in 2 hours", "finish report", 2, "H", "", false, false, "", 0},
		{"remind me weekly to water plants", "water plants", 0, "", "", false, true, "WEEKLY", 1},
		{"remind me every 2 weeks to clean the filter", "clean the filter", 14, "D", "", false, true, "WEEKLY", 2},
		{"remind me every day at 8pm to take vitamins", "take vitamins", 0, "", "8pm", false, true, "DAILY", 1},
		{"remind me tomorrow morning to check the car", "check the car", 1, "D", "", false, false, "", 0},
		{"set a reminder for a doctor appointment next week at 9:30am", "doctor appointment", 7, "D", "9:30am", false, false, "", 0},
		{"remind me in 45 days to pay bills -a", "pay bills", 45, "D", "", false, true, "DAILY", 45},
		{"renew gym membership every 2 months", "renew gym membership", 2, "M", "", false, true, "MONTHLY", 2},
		{"water plants daily without fail", "water plants", 0, "", "", false, true, "DAILY", 1},
	}
	for _, c := range cases {
		got, err := Parse(ctx, c.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got.Summary != c.summary {
			t.Errorf("Parse(%q) summary = %q, want %q", c.in, got.Summary, c.summary)
		}
		if got.Amount != c.amount || got.Unit != c.unit {
			t.Errorf("Parse(%q) time = (%v,%q), want (%v,%q)", c.in, got.Amount, got.Unit, c.amount, c.unit)
		}
		if got.TimeOfDay != c.tod {
			t.Errorf("Parse(%q) time_of_day = %q, want %q", c.in, got.TimeOfDay, c.tod)
		}
		if got.Critical != c.critical || got.Recurring != c.recur {
			t.Errorf("Parse(%q) flags = (crit:%v,recur:%v), want (crit:%v,recur:%v)",
				c.in, got.Critical, got.Recurring, c.critical, c.recur)
		}
		if got.Freq != c.freq || got.Interval != c.interval {
			t.Errorf("Parse(%q) recurrence = (%q,%d), want (%q,%d)",
				c.in, got.Freq, got.Interval, c.freq, c.interval)
		}
	}

	if _, err := Parse(ctx, "lorem ipsum dolor sit amet"); err == nil {
		t.Error("garbage input should error")
	}
}

func TestNormalizeRecurrence(t *testing.T) {
	oneOff := &EventIntent{Summary: "x"}
	oneOff.Normalize()
	if oneOff.Freq != "" {
		t.Errorf("one-off Normalize freq = %q, want empty", oneOff.Freq)
	}

	unitBased := &EventIntent{Summary: "x", Recurring: true, Unit: "M"}
	unitBased.Normalize()
	if unitBased.Freq != "MONTHLY" || unitBased.Interval != 1 {
		t.Errorf("unit-based Normalize = (%q,%d), want (MONTHLY,1)", unitBased.Freq, unitBased.Interval)
	}

	wordBased := &EventIntent{Summary: "x", Recurring: true}
	wordBased.Normalize()
	if wordBased.Freq != "DAILY" {
		t.Errorf("word-based default Normalize freq = %q, want DAILY", wordBased.Freq)
	}
}

func TestValidate(t *testing.T) {
	valid := []*EventIntent{
		{Summary: "x"},
		{Summary: "x", Amount: 1.5, Unit: "D", TimeOfDay: "5:30pm", Freq: "WEEKLY", Interval: 2},
	}
	for _, e := range valid {
		if err := e.Validate(); err != nil {
			t.Errorf("Validate(%+v) unexpected error: %v", e, err)
		}
	}
	invalid := []*EventIntent{
		{Summary: ""},
		{Summary: "x", Unit: "X"},
		{Summary: "x", Amount: -1, Unit: "D"},
		{Summary: "x", TimeOfDay: "25:99"},
		{Summary: "x", Freq: "EVERYTHING"},
		{Summary: "x", Interval: -2},
	}
	for _, e := range invalid {
		if err := e.Validate(); err == nil {
			t.Errorf("Validate(%+v) should have errored", e)
		}
	}
}
