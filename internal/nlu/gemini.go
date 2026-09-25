package nlu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"

// systemPrompt is a strict contract: the model may only ever return this exact
// JSON shape, with these exact field choices, and nothing else.
const systemPrompt = `You convert a casual reminder sentence into ONE JSON object. Return ONLY that JSON object: no markdown, no code fences, no prose, no explanations, no extra text.

The JSON must match EXACTLY this TypeScript shape. Never add fields, never omit required fields, never rename them:

interface ReminderIntent {
  // cleaned title of the reminder; drop filler ("remind me", "bro", "please", "a", "the")
  summary: string;
  // how far from NOW the reminder is, only when an offset is spoken
  // ("45 days", "in 30 minutes", "next month", "tomorrow"); null if none
  amount: number | null;
  // allowed: "D" | "M" | "Y" | "H" | "MIN" | ""
  unit: "D" | "M" | "Y" | "H" | "MIN" | "";
  // a clock time ONLY when spoken ("5:30pm", "2am", "14:30"); "" if none
  time_of_day: string;
  // repeat frequency ONLY when repetition is spoken; "" if it is a one-off:
  freq: "DAILY" | "WEEKLY" | "MONTHLY" | "YEARLY" | "HOURLY" | "MINUTELY" | "";
  // repeat every N units; 1 when N is not stated; null when one-off
  interval: number | null;
  // true only if the reminder repeats ("every day/week/month/year", "weekly", "-A")
  recurring: boolean;
  // true only if genuinely urgent: critical, urgent, important, emergency, or capitalized "Remind"
  critical: boolean;
}

Field rules:
- amount + unit: "in 30 minutes" -> amount 30, unit "MIN". "45 days from now" -> 45, "D". "in 2 hours" -> 2, "H". "next month" -> 1, "M". "tomorrow" -> 1, "D". A bare number alone means days.
- An absolute clock time ("at 5:30pm") goes in time_of_day and does NOT touch amount/unit.
- If both an offset and a clock time are spoken, set both.
- recurrence: "every day at 8pm" -> freq "DAILY", interval 1, time_of_day "8pm". "every 2 weeks" -> freq "WEEKLY", interval 2. "every month" / "monthly" -> "MONTHLY", 1. A repeat flag by itself (like -A) with an offset "45 days" -> freq "DAILY", interval 45. Do NOT set recurring just because a time was given.
- critical only for the urgent words above or a capital-R "Remind". A lowercase "remind" is not critical.
- If no time at all is mentioned, use amount null, unit "", time_of_day "", freq "", interval null and do not invent a time.

Examples:
Input: "remind to service my bike 45 days from now"
Output: {"summary":"service my bike","amount":45,"unit":"D","time_of_day":"","freq":"","interval":null,"recurring":false,"critical":false}

Input: "critical important meeting tomorrow at 5:30pm"
Output: {"summary":"important meeting","amount":1,"unit":"D","time_of_day":"5:30pm","freq":"","interval":null,"recurring":false,"critical":true}

Input: "remind me every month to pay rent"
Output: {"summary":"pay rent","amount":1,"unit":"M","time_of_day":"","freq":"MONTHLY","interval":1,"recurring":true,"critical":false}

Input: "remind me to call mom at 2am"
Output: {"summary":"call mom","amount":null,"unit":"","time_of_day":"2am","freq":"","interval":null,"recurring":false,"critical":false}`

// geminiRequest is the generateContent payload we send to Gemini.
type geminiRequest struct {
	SystemInstruction geminiContent  `json:"system_instruction"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  geminiConfig    `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiConfig struct {
	Temperature    float64         `json:"temperature"`
	ResponseMime   string          `json:"responseMimeType"`
	ResponseSchema json.RawMessage `json:"responseSchema,omitempty"`
	MaxOutputToks  int             `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var intentSchema = []byte(`{
  "type": "OBJECT",
  "properties": {
    "summary":     {"type": "STRING"},
    "amount":      {"type": "NUMBER", "nullable": true},
    "unit":        {"type": "STRING", "enum": ["D", "M", "Y", "H", "MIN"], "nullable": true},
    "time_of_day": {"type": "STRING", "nullable": true},
    "freq":        {"type": "STRING", "enum": ["DAILY", "WEEKLY", "MONTHLY", "YEARLY", "HOURLY", "MINUTELY"], "nullable": true},
    "interval":    {"type": "NUMBER", "nullable": true},
    "critical":    {"type": "BOOLEAN"},
    "recurring":   {"type": "BOOLEAN"}
  },
  "required": ["summary", "amount", "unit", "time_of_day", "freq", "interval", "critical", "recurring"]
}`)

// ParseGemini asks Gemini to turn text into an EventIntent. Transient API
// errors (5xx demand spikes) are retried, an optional GEMINI_MODEL_FALLBACK is
// tried before giving up, and invalid responses get one self-correcting retry.
// Quota (429) fails fast so the deterministic parser takes over immediately.
func ParseGemini(ctx context.Context, text string) (*EventIntent, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	models := []string{os.Getenv("GEMINI_MODEL")}
	if models[0] == "" {
		models[0] = "gemini-3.6-flash"
	}
	if fb := os.Getenv("GEMINI_MODEL_FALLBACK"); fb != "" {
		models = append(models, fb)
	}

	var lastErr error
	for _, model := range models {
		for attempt := 0; attempt < 3; attempt++ {
			intent, transient, err := generateIntent(ctx, key, model, text)
			if err == nil {
				return intent, nil
			}
			lastErr = err
			if !transient {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

func buildGeminiRequest(text string) geminiRequest {
	return geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Parts: []geminiPart{{Text: text}}}},
		GenerationConfig: geminiConfig{
			Temperature:    0.2,
			ResponseMime:   "application/json",
			ResponseSchema: intentSchema,
			MaxOutputToks:  300,
		},
	}
}

// generateIntent runs a primary attempt then, if the model returns something
// that does not validate, one corrected retry that names the exact error.
func generateIntent(ctx context.Context, key, model, text string) (*EventIntent, bool, error) {
	intent, transient, verr := parseIntentFrom(ctx, key, model, buildGeminiRequest(text))
	if transient {
		return nil, true, verr
	}
	if verr == nil {
		return intent, false, nil
	}

	repair := buildGeminiRequest(fmt.Sprintf(
		"Your previous answer for %q was rejected because: %v.\nReturn ONLY the corrected valid JSON object.",
		text, verr))
	intent, transient, verr = parseIntentFrom(ctx, key, model, repair)
	if verr != nil {
		return nil, transient, verr
	}
	return intent, false, nil
}

func parseIntentFrom(ctx context.Context, key, model string, req geminiRequest) (*EventIntent, bool, error) {
	raw, transient, err := callGemini(ctx, key, model, req)
	if err != nil {
		return nil, transient, err
	}
	intent := &EventIntent{}
	if err := json.Unmarshal([]byte(raw), intent); err != nil {
		return nil, false, fmt.Errorf("gemini returned invalid JSON: %w", err)
	}
	intent.Summary = strings.TrimSpace(intent.Summary)
	if err := intent.Validate(); err != nil {
		return nil, false, err
	}
	intent.Normalize()
	return intent, false, nil
}

func callGemini(ctx context.Context, key, model string, req geminiRequest) (string, bool, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", false, fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf(geminiEndpoint, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", key)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", true, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", true, fmt.Errorf("read gemini response: %w", err)
	}

	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", false, fmt.Errorf("parse gemini response: %w", err)
	}

	if parsed.Error != nil {
		transient := parsed.Error.Code >= 500
		return "", transient, fmt.Errorf("gemini error (%d): %s", parsed.Error.Code, parsed.Error.Message)
	}

	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", false, fmt.Errorf("gemini returned no content")
	}

	for _, part := range parsed.Candidates[0].Content.Parts {
		if t := cleanJSON(part.Text); strings.HasPrefix(t, "{") {
			return t, false, nil
		}
	}
	return "", false, fmt.Errorf("gemini returned no JSON object")
}

// cleanJSON strips code fences/whitespace so a JSON object can be unmarshaled.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}