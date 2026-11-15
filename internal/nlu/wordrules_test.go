package nlu

import (
	"context"
	"testing"
)

// Regression: "day after tomorrow" used to match "tomorrow" about 10% of the
// time because the phrase table was a Go map (random iteration order).
func TestWordRulesDeterministic(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in     string
		amount float64
		unit   string
	}{
		{"remind me day after tomorrow to pay rent", 2, "D"},
		{"remind me tomorrow to pay rent", 1, "D"},
		{"remind me next week to pay rent", 7, "D"},
		{"remind me next month to pay rent", 1, "M"},
	}
	for _, c := range cases {
		for i := 0; i < 200; i++ {
			got, err := ParseFallback(ctx, c.in)
			if err != nil {
				t.Fatalf("ParseFallback(%q) error: %v", c.in, err)
			}
			if got.Amount != c.amount || got.Unit != c.unit {
				t.Fatalf("run %d: ParseFallback(%q) = (%v,%q), want (%v,%q)",
					i, c.in, got.Amount, got.Unit, c.amount, c.unit)
			}
		}
	}
}

// \b boundaries: "now" must not match inside other words.
func TestWordRulesWordBoundary(t *testing.T) {
	got, err := ParseFallback(context.Background(), "remind me to check the snow tires")
	if err == nil && got.Unit != "" {
		t.Errorf("'snow' matched a relative-date rule: unit=%q amount=%v", got.Unit, got.Amount)
	}
}
