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
	reRecurring     = regexp.MustCompile(`(?i)\b(every|each|weekly|monthly|daily|yearly|annually|per\s+(day|week|month|year))\b`)
	reDashA         = regexp.MustCompile(`(?i)-a\b`)
	reCritical      = regexp.MustCompile(`(?i)\b(urgent|critical|important|asap|emergency|high\s+priority)\b`)
	reCriticalCap   = regexp.MustCompile(`\bRemind\b`)
	reFrequency     = regexp.MustCompile(`(?i)\b(?:every|each)\s+(?:\d+\s+)?(?:days?|weeks?|months?|years?|day|week|month|year|morning|afternoon|evening|night)\b`)
	reUnitsClause   = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:days?|d|years?|yrs?|y|months?|mos?|m|hours?|hrs?|h|minutes?|mins?|min|weeks?|wks?|w)\b\s*(?:from now|later|after|from|ahead)?`)
	reEveryCount    = regexp.MustCompile(`(?i)\b(every|each)\s+(?:(\d+)\s+)?(day|week|month|year|hour|minute)s?\b`)
	reFreqWord      = regexp.MustCompile(`(?i)\b(daily|weekly|monthly|yearly|annually)\b`)
	reEveryEach     = regexp.MustCompile(`(?i)\b(every|each)\b`)
	reDayPart       = regexp.MustCompile(`(?i)\b(morning|afternoon|evening|night)\b`)
	reFiller        = regexp.MustCompile(`(?i)\b(without\s+fail|don'?t\s+forget|make\s+sure|remember\s+to|remember|do\s+not\s+forget|no\s+matter\s+what)\b`)
)

var recurWordFreq = map[string]string{
	"day": "DAILY", "week": "WEEKLY", "month": "MONTHLY",
	"year": "YEARLY", "hour": "HOURLY", "minute": "MINUTELY",
}

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

	intent.Recurring = reRecurring.MatchString(lower) || reDashA.MatchString(text)
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

	setRecurrence(intent, lower)
	intent.Normalize()

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

// setRecurrence turns a recurring flag into an explicit RRULE frequency and
// interval: word-based ("every 2 weeks", "weekly") wins, then a bare iteration
// flag ("-A") carries the offset unit, otherwise it defaults to daily.
func setRecurrence(intent *EventIntent, lower string) {
	if !intent.Recurring {
		return
	}

	interval := 1
	if m := reEveryCount.FindStringSubmatch(lower); m != nil {
		if m[2] != "" {
			if n, err := strconv.Atoi(m[2]); err == nil && n > 0 {
				interval = n
			}
		}
		if f, ok := recurWordFreq[m[3]]; ok {
			intent.Freq = f
			intent.Interval = interval
			return
		}
	}

	if m := reFreqWord.FindStringSubmatch(lower); m != nil {
		switch strings.ToLower(m[1]) {
		case "daily":
			intent.Freq = "DAILY"
		case "weekly":
			intent.Freq = "WEEKLY"
		case "monthly":
			intent.Freq = "MONTHLY"
		case "yearly", "annually":
			intent.Freq = "YEARLY"
		}
		return
	}

	if intent.Amount > 0 {
		intent.Freq = UnitToFreq(intent.Unit)
		intent.Interval = int(intent.Amount)
		if intent.Interval < 1 {
			intent.Interval = 1
		}
	}
}

var reRemindVerb = regexp.MustCompile(`(?i)\b(remind|reminder|set\s*(a\s*)?reminder|remember)\b`)

var errNoSummary = &summaryError{}

type summaryError struct{}

func (*summaryError) Error() string { return "could not extract a summary from the phrase" }

var reCommand = regexp.MustCompile(`(?i)^\s*(hey|yo|bro|please|pls|okay|ok|alright|listen)?\s*(critical|urgent|important|asap|emergency|priority)?\s*(reminder|remind me to|remind me|remind us to|remind us|remind to|remind|set(?: up)?(?: a)?(?: reminder| remind)|put(?: a)?(?: reminder| remind)|schedule(?: a)?(?: reminder| remind)|add(?: a)?(?: reminder| remind)|need(?: a)?(?: reminder| remind)|want(?: a)?(?: reminder| remind))\s*(me|us)?\s*(to|for|about|of|regarding|on)?\s*`)
var reTrailing = regexp.MustCompile(`(?i)\b(in|at|to|for|from|the|a|an|my|your|our|of|on)\s*$`)
var reLeader = regexp.MustCompile(`(?i)^\s*(the|a|an|my|your|our|their|his|her|its|for|about|regarding|of|to|on|in)\b\s*`)

func stripSummary(text string) string {
	s := text

	// Remove the full duration clause ("45 days from now", "in 2 hours").
	s = reUnitsClause.ReplaceAllString(s, " ")
	s = reTodayTomorrow.ReplaceAllStringFunc(s, func(string) string { return " " })
	s = reTimeOfDay.ReplaceAllString(s, " ")
	s = reFrequency.ReplaceAllString(s, " ")
	s = reRecurring.ReplaceAllString(s, " ")
	s = reDashA.ReplaceAllString(s, " ")
	s = reEveryEach.ReplaceAllString(s, " ")
	s = reDayPart.ReplaceAllString(s, " ")
	s = reFiller.ReplaceAllString(s, " ")

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

	// Drop leftover leading articles/prepositions ("for a doctor appointment").
	for prev := ""; prev != s; {
		prev = s
		s = reLeader.ReplaceAllString(s, " ")
		s = reTrailing.ReplaceAllString(s, " ")
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}