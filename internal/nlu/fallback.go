package nlu

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// Fallback parser: a deterministic regex parser that mirrors gcal's command
// grammar ("in 45 days", "tomorrow 5:30pm", "every month", "-A", "Remind",
// "at 2am"). Used when Gemini is unavailable or rate-limited so the app still
// works.

var fallbackPatterns = []struct {
	re   *regexp.Regexp
	unit string
	mult float64
}{
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:days?|d)\b`), "D", 1},
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:years?|yrs?|y)\b`), "Y", 1},
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:months?|mos?|m)\b`), "M", 1},
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:hours?|hrs?|hr|h)\b`), "H", 1},
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:minutes?|mins?|min)\b`), "MIN", 1},
	{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:weeks?|wks?|w)\b`), "D", 7},
}

var (
	reTodayTomorrow = regexp.MustCompile(`\b(day after tomorrow|tomorrow|next week|tonight|today|now|next month|next year)\b`)
	reTimeOfDay     = regexp.MustCompile(`(?i)(?:at|around|by|before)\s+(\d{1,2}(?::\d{2})?\s*(?:am|pm)|\d{2}:\d{2})`)
	reRecurring     = regexp.MustCompile(`(?i)\b(every|each|weekly|monthly|daily|yearly|annually|\-A|per\s+(day|week|month|year))\b`)
	reCritical      = regexp.MustCompile(`(?i)\b(urgent|critical|important|asap|emergency|high\s+priority)\b`)
	reCriticalCap   = regexp.MustCompile(`\bRemind\b`)
	reFrequency     = regexp.MustCompile(`(?i)\b(?:every|each)\s+(?:\d+\s+)?(?:days?|weeks?|months?|years?|day|week|month|year|morning|afternoon|evening|night)\b`)
	reUnitsClause   = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:days?|d|years?|yrs?|y|months?|mos?|m|hours?|hrs?|h|minutes?|mins?|min|weeks?|wks?|w)\b\s*(?:from now|later|after|from|ahead)?`)
)

var wordMap = map[string]struct {
	amount float64
	unit   string
	tod    string
}{
	"day after tomorrow": {2, "D", ""},
	"tomorrow":           {1, "D", ""},
	"next week":          {7, "D", ""},
	"tonight":            {0, "", "8pm"},
	"today":              {0, "", ""},
	"now":                {0, "", ""},
	"next month":         {1, "M", ""},
	"next year":          {1, "Y", ""},
}

// ParseFallback deterministically parses a reminder phrase. For best-effort
// use only; returns an error when it cannot find enough structure.
func ParseFallback(_ context.Context, text string) (*EventIntent, error) {
	lower := strings.ToLower(text)
	intent := &EventIntent{}

	intent.Recurring = reRecurring.MatchString(lower)
	intent.Critical = reCritical.MatchString(lower) || reCriticalCap.MatchString(text)

	// Time of day.
	if m := reTimeOfDay.FindStringSubmatch(text); m != nil {
		intent.TimeOfDay = strings.TrimSpace(strings.ToLower(m[1]))
	}

	// Duration.
	for _, p := range fallbackPatterns {
		if m := p.re.FindStringSubmatch(lower); m != nil {
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			intent.Amount = v * p.mult
			intent.Unit = p.unit
			break
		}
	}

	// Word-based dates (only if no explicit duration was found).
	if intent.Unit == "" {
		for word, w := range wordMap {
			if strings.Contains(lower, word) {
				intent.Amount = w.amount
				intent.Unit = w.unit
				if w.tod != "" {
					intent.TimeOfDay = w.tod
				}
				break
			}
		}
	}

	// Recurring monthly with no explicit "in X": default to 1M (matches -A).
	if intent.Recurring && intent.Unit == "" && (strings.Contains(lower, "month") || strings.Contains(lower, "-a")) {
		intent.Amount = 1
		intent.Unit = "M"
	}

	intent.Summary = stripSummary(text)
	if intent.Summary == "" {
		return nil, errNoSummary
	}

	hasTime := intent.Amount > 0 || intent.TimeOfDay != ""
	if !hasTime && !intent.Recurring && !intent.Critical && !reRemindVerb.MatchString(text) {
		return nil, errNoSummary
	}
	return intent, nil
}

var reRemindVerb = regexp.MustCompile(`(?i)\b(remind|reminder|set\s*(a\s*)?reminder|remember)\b`)

var errNoSummary = &summaryError{}

type summaryError struct{}

func (*summaryError) Error() string { return "could not extract a summary from the phrase" }

var reCommand = regexp.MustCompile(`(?i)^\s*(hey|yo|bro|please|pls|okay|ok)?\s*(critical|urgent|important|asap)?\s*(remind|reminder|remind me|remind me to|remind to|set a reminder|set reminder)\s*(me|us)?\s*(to|for|about|of)?\s*`)
var reTrailing = regexp.MustCompile(`(?i)\b(in|at|to|for|from|the|a|an|my|your|our|of|on)\s*$`)

func stripSummary(text string) string {
	s := text

	// Remove the full duration clause ("45 days from now", "in 2 hours").
	s = reUnitsClause.ReplaceAllString(s, " ")
	s = reTodayTomorrow.ReplaceAllStringFunc(s, func(string) string { return " " })
	s = reTimeOfDay.ReplaceAllString(s, " ")
	s = reFrequency.ReplaceAllString(s, " ")

	// Remove leading command/filler words (repeat to eat multi-part verbs).
	for prev := ""; prev != s; {
		prev = s
		s = reCommand.ReplaceAllString(s, " ")
	}

	s = reCritical.ReplaceAllString(s, " ")

	// Drop trailing prepositions/stopwords left by stripped time clauses.
	for prev := ""; prev != s; {
		prev = s
		s = reTrailing.ReplaceAllString(s, " ")
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}