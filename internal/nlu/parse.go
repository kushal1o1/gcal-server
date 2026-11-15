package nlu

import (
	"context"
	"errors"
	"time"

	"github.com/kushal1o1/gcal-svr/internal/timing"
)

// Parse turns a reminder phrase into an EventIntent using the deterministic
// regex parser. The bool is kept for API compatibility and is always true:
// the fallback parser is now the only parser.
func Parse(ctx context.Context, text string) (*EventIntent, bool, error) {
	t := time.Now()
	intent, err := ParseFallback(ctx, text)
	timing.Since("nlu", t)
	if err != nil {
		return nil, true, err
	}
	return intent, true, nil
}

// IsUnparseable reports whether an inner fallback error means the phrase had
// no extractable structure at all.
func IsUnparseable(err error) bool {
	var se *summaryError
	return errors.As(err, &se)
}