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

const systemPrompt = `You convert a person's casual reminder sentence into a strict JSON object. Only output JSON, nothing else.

Rules:
- summary: the thing to be reminded about, cleaned up (no filler words like "remind me", "bro", "please").
- amount + unit: how long from NOW until the reminder. Unit is one of: D (days), M (months), Y (years), H (hours), MIN (minutes). If the phrase is an absolute time today ("at 5:30pm") leave amount null and unit empty string; put the time in time_of_day.
- time_of_day: a 12 or 24 hour clock time if mentioned ("5:30pm", "2am", "14:30"), otherwise empty string.
- recurring: true if the reminder repeats (words like every day/week/month/year, weekly, monthly, "-A").
- critical: true if it sounds urgent (words like critical, urgent, important, emergency, critical="Remind").
- If no time is mentioned at all for a one-off, leave amount null, unit "", time_of_day "" (defaults to today).

Examples:
Input:  "remind to service my bike 45 days from now"
Output: {"summary":"service my bike","amount":45,"unit":"D","time_of_day":"","critical":false,"recurring":false}

Input:  "critical, important meeting tomorrow at 5:30pm"
Output: {"summary":"important meeting","amount":1,"unit":"D","time_of_day":"5:30pm","critical":true,"recurring":false}

Input:  "remind me every month to pay rent"
Output: {"summary":"pay rent","amount":1,"unit":"M","time_of_day":"","critical":false,"recurring":true}

Input:  "bro remind me to buy milk"
Output: {"summary":"buy milk","amount":null,"unit":"","time_of_day":"","critical":false,"recurring":false}

Input:  "Remind me to call mom at 2am"
Output: {"summary":"call mom","amount":null,"unit":"","time_of_day":"2am","critical":true,"recurring":false}

Input:  "remind to take medicine in 30 minutes"
Output: {"summary":"take medicine","amount":30,"unit":"MIN","time_of_day":"","critical":false,"recurring":false}`

// geminiSchemaResponse is the OpenAI/Gemini style response schema requested
// via responseMimeType=application/json.
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
    "critical":    {"type": "BOOLEAN"},
    "recurring":   {"type": "BOOLEAN"}
  },
  "required": ["summary", "amount", "unit", "time_of_day", "critical", "recurring"]
}`)

// ParseGemini asks Gemini to turn text into an EventIntent. Transient API
// errors (429 rate limits, 5xx demand spikes) are retried, and an optional
// GEMINI_MODEL_FALLBACK is tried before giving up.
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

func generateIntent(ctx context.Context, key, model, text string) (*EventIntent, bool, error) {

	req := geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Parts: []geminiPart{{Text: text}}}},
		GenerationConfig: geminiConfig{
			Temperature:    0.2,
			ResponseMime:   "application/json",
			ResponseSchema: intentSchema,
			MaxOutputToks:  200,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, false, fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf(geminiEndpoint, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", key)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, true, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, true, fmt.Errorf("read gemini response: %w", err)
	}

	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, false, fmt.Errorf("parse gemini response: %w", err)
	}

	if parsed.Error != nil {
		transient := parsed.Error.Code == 429 || parsed.Error.Code >= 500
		return nil, transient, fmt.Errorf("gemini error (%d): %s", parsed.Error.Code, parsed.Error.Message)
	}

	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, false, fmt.Errorf("gemini returned no content")
	}

	raw := parsed.Candidates[0].Content.Parts[0].Text
	intent := &EventIntent{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), intent); err != nil {
		return nil, false, fmt.Errorf("gemini returned invalid JSON %q: %w", raw, err)
	}

	intent.Summary = strings.TrimSpace(intent.Summary)
	if intent.Summary == "" {
		return nil, false, fmt.Errorf("gemini did not extract a summary")
	}
	return intent, false, nil
}