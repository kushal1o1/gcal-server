package nlu

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Parser: a deterministic regex grammar for reminder phrases. It reads the
// timing, repeat, importance and weekday parts of a sentence, and whatever is
// left over becomes the reminder text.

var durationPatterns = []struct {
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
	reRecurring     = regexp.MustCompile(`(?i)\b(every|each|always|daily|weekly|monthly|yearly|annually|regularly|repeat|repeats|repeatedly|recurring|per\s+(day|week|month|year))\b`)
	reDashA         = regexp.MustCompile(`(?i)-a\b`)
	reCritical      = regexp.MustCompile(`(?i)\b(urgent|urgently|critical|important|asap|emergency|high\s+priority|priority|deadline|don'?t\s+miss|do\s+not\s+miss|really\s+important)\b`)
	reCriticalCap   = regexp.MustCompile(`\bRemind\b`)
	reFrequency     = regexp.MustCompile(`(?i)\b(?:every|each)\s+(?:\d+\s+)?(?:days?|weeks?|months?|years?|hours?|hrs?|minutes?|mins?|day|week|month|year|hour|minute|morning|afternoon|evening|night)\b`)
	reUnitsClause   = regexp.MustCompile(`(?:\b(?:in|for|after|within|about|around)\s+)?(\d+(?:\.\d+)?)\s*(?:days?|d|years?|yrs?|y|months?|mos?|m|hours?|hrs?|h|minutes?|mins?|min|weeks?|wks?|w)\b\s*(?:from now|later|after|from|ahead)?`)
	reEveryCount    = regexp.MustCompile(`(?i)\b(every|each)\s+(?:(\d+)\s+)?(day|week|month|year|hour|minute)s?\b`)
	reFreqWord      = regexp.MustCompile(`(?i)\b(daily|weekly|monthly|yearly|annually)\b`)
	reEveryEach     = regexp.MustCompile(`(?i)\b(every|each)\b`)
	reDayPart       = regexp.MustCompile(`(?i)\b(morning|afternoon|evening|night)\b`)
	reFiller        = regexp.MustCompile(`(?i)\b(this\s+is|it\s+is|it's|its|without\s+fail|don'?t\s+forget|make\s+sure|remember\s+to|remember|do\s+not\s+forget|no\s+matter\s+what)\b`)
)

var recurWordFreq = map[string]string{
	"day": "DAILY", "week": "WEEKLY", "month": "MONTHLY",
	"year": "YEARLY", "hour": "HOURLY", "minute": "MINUTELY",
}

// wordRule is one relative-date phrase. Matching is deterministic: the rules
// are sorted longest phrase first, so "day after tomorrow" always beats
// "tomorrow". (The old map-based lookup iterated in random order.)
type wordRule struct {
	phrase string
	re     *regexp.Regexp
	amount float64
	unit   string
	tod    string
}

var wordRules = buildWordRules([]wordRule{
	{phrase: "day after tomorrow", amount: 2, unit: "D"},
	{phrase: "next month", amount: 1, unit: "M"},
	{phrase: "next year", amount: 1, unit: "Y"},
	{phrase: "next week", amount: 7, unit: "D"},
	{phrase: "tomorrow", amount: 1, unit: "D"},
	{phrase: "tonight", tod: "8pm"},
	{phrase: "today"},
	{phrase: "now"},
})

func buildWordRules(rules []wordRule) []wordRule {
	sort.SliceStable(rules, func(i, j int) bool {
		return len(rules[i].phrase) > len(rules[j].phrase)
	})
	for i := range rules {
		// \b keeps "now" from matching inside "know" or "snow".
		rules[i].re = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(rules[i].phrase) + `\b`)
	}
	return rules
}

// Parse deterministically parses a reminder phrase. For best-effort
// use only; returns an error when it cannot find enough structure.
func Parse(_ context.Context, text string) (*EventIntent, error) {
	text = normalizeNumbers(text)
	lower := strings.ToLower(text)
	intent := &EventIntent{}

	intent.Recurring = reRecurring.MatchString(lower) || reDashA.MatchString(text)
	intent.Critical = reCritical.MatchString(lower) || reCriticalCap.MatchString(text)

	// Time of day.
	if m := reTimeOfDay.FindStringSubmatch(text); m != nil {
		intent.TimeOfDay = strings.TrimSpace(strings.ToLower(m[1]))
	} else if tod, ok := bareHourToTime(text); ok {
		intent.TimeOfDay = tod
	}

	// Duration. Compound ("2 hours 30 minutes") is checked first, since the
	// single-unit patterns would otherwise take only the first half.
	if a, u, ok := parseCompound(lower); ok {
		intent.Amount, intent.Unit = a, u
	}
	for _, p := range durationPatterns {
		if intent.Unit != "" {
			break
		}
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
		for _, w := range wordRules {
			if w.re.MatchString(lower) {
				intent.Amount = w.amount
				intent.Unit = w.unit
				if w.tod != "" {
					intent.TimeOfDay = w.tod
				}
				break
			}
		}
	}

	// "every 45" with no unit: refuse to guess between days, weeks or months.
	if m := reEveryNumber.FindStringSubmatch(lower); m != nil && intent.Unit == "" {
		return nil, fmt.Errorf("\"every %s\" needs a unit: days, weeks, months, years, hours or minutes", m[1])
	}

	// Weekdays: "every monday" repeats weekly, "on friday" means the next Friday.
	if m := reWeekday.FindStringSubmatch(lower); m != nil {
		intent.ByDay = weekdayCode(m[2])
		if m[1] == "every" || m[1] == "each" {
			intent.Recurring = true
		}
	}

	setRecurrence(intent, lower)
	if intent.ByDay != "" && intent.Recurring {
		intent.Freq = "WEEKLY"
		intent.Interval = 1
	}
	intent.Normalize()

	summaryText := text
	if intent.TimeOfDay != "" && !reTimeOfDay.MatchString(text) {
		summaryText = reBareHour.ReplaceAllString(text, " ")
	}
	intent.Summary = stripSummary(summaryText)
	if intent.Summary == "" {
		return nil, errNoSummary
	}

	hasTime := intent.Amount > 0 || intent.TimeOfDay != "" || intent.ByDay != ""
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
var reWeekday = regexp.MustCompile(`(?i)\b(?:(every|each|on|next|this|coming)\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday)s?\b`)

// reEveryNumber catches "every 45" with no unit after it.
var reEveryNumber = regexp.MustCompile(`(?i)\b(?:every|each)\s+(\d+(?:\.\d+)?)\b`)

var rePunctLead = regexp.MustCompile(`^[\s,:;.!?-]+`)

// reBareHour matches "at 5" or "at 8:30" with no am/pm.
var reBareHour = regexp.MustCompile(`(?i)\b(?:at|by|around)\s+(\d{1,2})(?::(\d{2}))?\b`)

var reTrailing = regexp.MustCompile(`(?i)(?:^|\s)(in|at|to|for|from|the|a|an|my|your|our|of|on|by|before|until|every|each|next|this)[\s,.;:!?-]*$`)
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
	s = reWeekday.ReplaceAllString(s, " ")

	s = reCritical.ReplaceAllString(s, " ")
	s = rePunctLead.ReplaceAllString(s, "")

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
	return strings.Trim(strings.Join(fields, " "), " :,-.!?;")
}

// weekdayCode maps an English weekday name to its iCalendar BYDAY code.
func weekdayCode(name string) string {
	switch strings.ToLower(name) {
	case "monday":
		return "MO"
	case "tuesday":
		return "TU"
	case "wednesday":
		return "WE"
	case "thursday":
		return "TH"
	case "friday":
		return "FR"
	case "saturday":
		return "SA"
	case "sunday":
		return "SU"
	}
	return ""
}

// bareHourToTime turns "at 5" into "5pm" when no am/pm is given. The rule is
// fixed: 7-11 is morning, 12 is noon, 1-6 is afternoon. Phrases where the hour
// is followed by a unit ("at 5 days") are not times and are skipped.
func bareHourToTime(text string) (string, bool) {
	loc := reBareHour.FindStringSubmatchIndex(text)
	if loc == nil {
		return "", false
	}
	rest := strings.Fields(strings.ToLower(text[loc[1]:]))
	if len(rest) > 0 {
		switch strings.Trim(rest[0], ".,!?") {
		case "am", "pm", "a.m.", "p.m.":
			return "", false // explicit meridiem; reTimeOfDay handles it
		case "day", "days", "week", "weeks", "month", "months", "year", "years",
			"hour", "hours", "hr", "hrs", "min", "mins", "minute", "minutes":
			return "", false
		}
	}
	m := reBareHour.FindStringSubmatch(text)
	hour, err := strconv.Atoi(m[1])
	if err != nil || hour < 1 || hour > 12 {
		return "", false
	}
	suffix := "pm"
	if hour >= 7 && hour <= 11 {
		suffix = "am"
	}
	if m[2] != "" {
		return fmt.Sprintf("%d:%s%s", hour, m[2], suffix), true
	}
	return fmt.Sprintf("%d%s", hour, suffix), true
}
