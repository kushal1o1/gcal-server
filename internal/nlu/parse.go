package nlu

import (
	"context"
	"errors"
	"fmt"
)

// Parse tries Gemini first and falls back to the deterministic parser when the
// model is unavailable or rate-limited. Returns whether the fallback was used.
func Parse(ctx context.Context, text string) (*EventIntent, bool, error) {
	intent, err := ParseGemini(ctx, text)
	if err == nil {
		return intent, false, nil
	}

	fallbackIntent, fallbackErr := ParseFallback(ctx, text)
	if fallbackErr != nil {
		return nil, false, fmt.Errorf("nlu failed (gemini: %v; fallback: %v)", err, fallbackErr)
	}
	return fallbackIntent, true, nil
}

// IsUnparseable reports whether an inner fallback error means the phrase had
// no extractable structure at all.
func IsUnparseable(err error) bool {
	var se *summaryError
	return errors.As(err, &se)
}