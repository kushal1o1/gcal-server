package nlu

import (
	"regexp"
	"strconv"
	"strings"
)

// Number-word support for the parser: "three days" -> "3 days",
// "in a week" -> "in 1 week", "half an hour" -> "30 minutes", and compound
// durations like "2 hours 30 minutes" -> 150 MIN.

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	"fifteen": 15, "twenty": 20, "thirty": 30, "forty": 40, "fifty": 50,
	"sixty": 60, "ninety": 90,
}

const durationUnitWords = `days?|weeks?|months?|years?|hours?|hrs?|minutes?|mins?`

var (
	reNumberWord = regexp.MustCompile(`(?i)\b(` + wordAlternation() + `)\s+(` + durationUnitWords + `)\b`)
	reArticle    = regexp.MustCompile(`(?i)\b(in|for|after|within|about|around)\s+(?:a|an)\s+(` + durationUnitWords + `)\b`)
	reHalfHour   = regexp.MustCompile(`(?i)\bhalf\s+(?:an?\s+)?hour\b`)
	reCompound   = regexp.MustCompile(`(?i)\b(\d+)\s*(?:hours?|hrs?|hr|h)\s*(?:and\s*)?(\d+)\s*(?:minutes?|mins?|min)\b`)
	reCompAnd    = regexp.MustCompile(`(?i)\b(\d+\s*(?:hours?|hrs?|hr|h))\s+and\s+(\d+\s*(?:minutes?|mins?|min))\b`)
)

func wordAlternation() string {
	words := make([]string, 0, len(numberWords))
	for w := range numberWords {
		words = append(words, w)
	}
	return strings.Join(words, "|")
}

// normalizeNumbers rewrites spelled-out quantities into digits so the regex
// grammar only has to handle one form.
func normalizeNumbers(text string) string {
	s := reCompAnd.ReplaceAllString(text, "$1 $2")
	s = reHalfHour.ReplaceAllString(s, "30 minutes")
	s = reArticle.ReplaceAllString(s, "$1 1 $2")
	s = reNumberWord.ReplaceAllStringFunc(s, func(m string) string {
		parts := reNumberWord.FindStringSubmatch(m)
		n := numberWords[strings.ToLower(parts[1])]
		return strconv.Itoa(n) + " " + parts[2]
	})
	return s
}

// parseCompound handles "2 hours 30 minutes" and returns the total in minutes.
func parseCompound(lower string) (float64, string, bool) {
	m := reCompound.FindStringSubmatch(lower)
	if m == nil {
		return 0, "", false
	}
	h, errH := strconv.Atoi(m[1])
	mins, errM := strconv.Atoi(m[2])
	if errH != nil || errM != nil {
		return 0, "", false
	}
	return float64(h*60 + mins), "MIN", true
}
