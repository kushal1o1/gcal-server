package nlu

import (
	"context"
	"testing"
)

func TestParseFallback(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in       string
		summary  string
		amount   float64
		unit     string
		tod      string
		critical bool
		recur    bool
	}{
		{"remind to service my bike 45 days from now", "service my bike", 45, "D", "", false, false},
		{"remind me every month to pay rent", "pay rent", 1, "M", "", false, true},
		{"critical important meeting tomorrow at 5:30pm", "meeting", 1, "D", "5:30pm", true, false},
		{"Remind me to call mom at 2am", "call mom", 0, "", "2am", true, false},
		{"remind to take medicine in 30 minutes", "take medicine", 30, "MIN", "", false, false},
		{"bro remind me to buy milk", "buy milk", 0, "", "", false, false},
		{"remind me of bike service next week", "bike service", 7, "D", "", false, false},
		{"finish report in 2 hours", "finish report", 2, "H", "", false, false},
	}
	for _, c := range cases {
		got, err := ParseFallback(ctx, c.in)
		if err != nil {
			t.Errorf("ParseFallback(%q) error: %v", c.in, err)
			continue
		}
		if got.Summary != c.summary {
			t.Errorf("ParseFallback(%q) summary = %q, want %q", c.in, got.Summary, c.summary)
		}
		if got.Amount != c.amount || got.Unit != c.unit {
			t.Errorf("ParseFallback(%q) time = (%v,%q), want (%v,%q)", c.in, got.Amount, got.Unit, c.amount, c.unit)
		}
		if got.TimeOfDay != c.tod {
			t.Errorf("ParseFallback(%q) time_of_day = %q, want %q", c.in, got.TimeOfDay, c.tod)
		}
		if got.Critical != c.critical || got.Recurring != c.recur {
			t.Errorf("ParseFallback(%q) flags = (crit:%v,recur:%v), want (crit:%v,recur:%v)",
				c.in, got.Critical, got.Recurring, c.critical, c.recur)
		}
	}

	if _, err := ParseFallback(ctx, "lorem ipsum dolor sit amet"); err == nil {
		t.Error("garbage input should error")
	}
}