package timeparse

import (
	"testing"
	"time"
)

func TestParseTimeUnit(t *testing.T) {
	cases := []struct {
		in      string
		wantVal float64
		wantU   string
	}{
		{"10hrs", 10, "H"},
		{"10min", 10, "MIN"},
		{"5D", 5, "D"},
		{"2.5hours", 2.5, "H"},
		{"3.2D", 3.2, "D"},
		{"2hr", 2, "H"},
		{"30minutes", 30, "MIN"},
		{"1M", 1, "M"},
		{"1Y", 1, "Y"},
		{"7", 7, "D"},
	}
	for _, c := range cases {
		gotVal, gotU, err := ParseTimeUnit(c.in)
		if err != nil {
			t.Errorf("ParseTimeUnit(%q) unexpected error: %v", c.in, err)
			continue
		}
		if gotVal != c.wantVal || gotU != c.wantU {
			t.Errorf("ParseTimeUnit(%q) = (%v, %q), want (%v, %q)", c.in, gotVal, gotU, c.wantVal, c.wantU)
		}
	}

	if _, _, err := ParseTimeUnit("invalid"); err == nil {
		t.Error("ParseTimeUnit(\"invalid\") should error")
	}
}

func TestCalculateFutureDate(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, AppTZ())

	if got, _ := CalculateFutureDate(2, "D", base); got.Day() != 3 || got.Month() != 1 || got.Year() != 2025 {
		t.Errorf("D case wrong: %v", got)
	}
	if got, _ := CalculateFutureDate(5, "H", base); got.Hour() != 5 {
		t.Errorf("H case wrong: %v", got)
	}
	if got, _ := CalculateFutureDate(30, "MIN", base); got.Minute() != 30 {
		t.Errorf("MIN case wrong: %v", got)
	}
	if got, _ := CalculateFutureDate(2, "M", base); got.Month() != 3 || got.Year() != 2025 {
		t.Errorf("M case wrong: %v", got)
	}
	if got, _ := CalculateFutureDate(1, "Y", base); got.Year() != 2026 {
		t.Errorf("Y case wrong: %v", got)
	}

	if _, err := CalculateFutureDate(-1, "D", base); err == nil {
		t.Error("negative value should error")
	}
	if _, err := CalculateFutureDate(2, "X", base); err == nil {
		t.Error("invalid unit should error")
	}
}

func TestCalculateFutureDateLeapYear(t *testing.T) {
	base := time.Date(2024, 2, 29, 0, 0, 0, 0, AppTZ())
	got, _ := CalculateFutureDate(1, "Y", base)
	if got.Month() != 2 || got.Day() != 28 || got.Year() != 2025 {
		t.Errorf("leap year Y case wrong: %v", got)
	}
}

func TestCalculateFutureDateMonthOverflow(t *testing.T) {
	base := time.Date(2025, 1, 31, 0, 0, 0, 0, AppTZ())
	got, _ := CalculateFutureDate(1, "M", base)
	if got.Month() != 2 || got.Day() != 28 {
		t.Errorf("month overflow case wrong: %v", got)
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		in           string
		wantH, wantM int
	}{
		{"14:30", 14, 30},
		{"00:00", 0, 0},
		{"2:30am", 2, 30},
		{"2:30pm", 14, 30},
		{"12:00am", 0, 0},
		{"12:00pm", 12, 0},
		{"2am", 2, 0},
	}
	for _, c := range cases {
		gotH, gotM, err := ParseTime(c.in)
		if err != nil {
			t.Errorf("ParseTime(%q) unexpected error: %v", c.in, err)
			continue
		}
		if gotH != c.wantH || gotM != c.wantM {
			t.Errorf("ParseTime(%q) = (%d,%d), want (%d,%d)", c.in, gotH, gotM, c.wantH, c.wantM)
		}
	}

	for _, bad := range []string{"25:00", "14:60", "invalid"} {
		if _, _, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%q) should error", bad)
		}
	}
}
