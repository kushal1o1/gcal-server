// Package timeparse is a Go port of gcal's time_parser.py.
package timeparse

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // embed the zone database so TIMEZONE works on minimal runtimes
)

// DefaultTimezone is used when TIMEZONE is not set.
const DefaultTimezone = "Asia/Kathmandu"

var (
	appTZOnce sync.Once
	appTZ     *time.Location
)

// AppTZ is the time zone for all calendar math and event times. It comes from
// the TIMEZONE env var (an IANA name such as "Europe/London") and defaults to
// Asia/Kathmandu. It is read on first use, so a .env file loaded in main works.
func AppTZ() *time.Location {
	appTZOnce.Do(func() {
		name := os.Getenv("TIMEZONE")
		if name == "" {
			name = DefaultTimezone
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			log.Printf("timezone: invalid TIMEZONE %q, using %s: %v", name, DefaultTimezone, err)
			loc, _ = time.LoadLocation(DefaultTimezone)
		}
		appTZ = loc
	})
	return appTZ
}

type unitPattern struct {
	re   *regexp.Regexp
	unit string
}

var unitPatterns = []unitPattern{
	{regexp.MustCompile(`^(\d+(?:\.\d+)?)(?:hrs?|h|hours?)$`), "H"},
	{regexp.MustCompile(`^(\d+(?:\.\d+)?)(?:mins?|minutes?)$`), "MIN"},
	{regexp.MustCompile(`^(\d+(?:\.\d+)?)([dmy])$`), ""},
}

// ParseTimeUnit parses "2D", "3M", "1Y", "2hr", "30min" into (value, unit).
// Mirrors gcal's parse_time_unit, including the bare-number fallback to days.
func ParseTimeUnit(timeStr string) (float64, string, error) {
	s := strings.ToLower(strings.TrimSpace(timeStr))

	for _, p := range unitPatterns {
		m := p.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		value, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, "", fmt.Errorf("invalid time unit format: %s", timeStr)
		}
		unit := p.unit
		if unit == "" {
			unit = strings.ToUpper(m[2])
		}
		return value, unit, nil
	}

	if value, err := strconv.ParseFloat(s, 64); err == nil {
		return value, "D", nil
	}
	return 0, "", fmt.Errorf("invalid time unit format: %s", timeStr)
}

// CalculateFutureDate returns base (default now in Nepal TZ) advanced by value
// of unit (D/M/Y/H/MIN). Mirrors gcal's calculate_future_date, including
// fractional-month math and the Feb 29 -> Feb 28 leap-year guard.
func CalculateFutureDate(value float64, unit string, base time.Time) (time.Time, error) {
	if base.IsZero() {
		base = time.Now().In(AppTZ())
	}
	if value < 0 {
		return time.Time{}, fmt.Errorf("negative time values not supported: %v", value)
	}

	switch unit {
	case "D":
		return base.Add(time.Duration(value * 24 * float64(time.Hour))), nil
	case "M":
		wholeMonths := int(value)
		remainingDays := (value - float64(wholeMonths)) * 30

		totalMonths := int(base.Month()) - 1 + wholeMonths
		year := base.Year() + totalMonths/12
		month := time.Month(totalMonths%12 + 1)

		monthAfter := monthStart(year, month).AddDate(0, 1, 0)
		daysInMonth := monthAfter.Sub(monthStart(year, month)).Hours() / 24
		day := int(base.Day())
		if day > int(daysInMonth) {
			day = int(daysInMonth)
		}

		result := time.Date(year, month, day, base.Hour(), base.Minute(), base.Second(), 0, base.Location())
		return result.Add(time.Duration(remainingDays * 24 * float64(time.Hour))), nil
	case "Y":
		year := base.Year() + int(value)
		day := int(base.Day())
		if base.Month() == time.February && day == 29 && !isLeapYear(year) {
			day = 28
		}
		return time.Date(year, base.Month(), day, base.Hour(), base.Minute(), base.Second(), 0, base.Location()), nil
	case "H":
		return base.Add(time.Duration(value * float64(time.Hour))), nil
	case "MIN":
		return base.Add(time.Duration(value * float64(time.Minute))), nil
	default:
		return time.Time{}, fmt.Errorf("invalid time unit: %s", unit)
	}
}

func monthStart(year int, month time.Month) time.Time {
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

func isLeapYear(year int) bool {
	if year%400 == 0 {
		return true
	}
	if year%100 == 0 {
		return false
	}
	return year%4 == 0
}

var time24 = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
var time12 = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)$`)

// ParseTime parses "14:30", "2am", "2:30pm" into (hour, minute) 24h. Mirrors
// gcal's parse_time.
func ParseTime(timeStr string) (int, int, error) {
	s := strings.ToLower(strings.TrimSpace(timeStr))

	if m := time24.FindStringSubmatch(s); m != nil {
		hour, _ := strconv.Atoi(m[1])
		minute, _ := strconv.Atoi(m[2])
		if hour < 24 && minute < 60 {
			return hour, minute, nil
		}
	}

	if m := time12.FindStringSubmatch(s); m != nil {
		hour, _ := strconv.Atoi(m[1])
		minute := 0
		if m[2] != "" {
			minute, _ = strconv.Atoi(m[2])
		}
		meridian := m[3]

		if !(hour > 0 && hour <= 12 && minute < 60) {
			return 0, 0, fmt.Errorf("invalid time format: %s", timeStr)
		}

		if meridian == "pm" && hour != 12 {
			hour += 12
		} else if meridian == "am" && hour == 12 {
			hour = 0
		}
		return hour, minute, nil
	}

	return 0, 0, fmt.Errorf("invalid time format: %s", timeStr)
}
