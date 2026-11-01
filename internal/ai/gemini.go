package ai

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/genai"
)

// GenerateFunc sends one prompt to a Gemini model and returns its text.
type GenerateFunc func(ctx context.Context, model, prompt string) (string, error)

type gemini struct {
	gen     GenerateFunc
	model   string
	backoff []time.Duration
}

var defaultGeminiBackoff = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

// NewGemini returns the Gemini fallback provider. An empty key is an error; the caller then runs Claude only.
func NewGemini(apiKey, model string) (Provider, error) {
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY not set")
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	gen := func(ctx context.Context, model, prompt string) (string, error) {
		resp, err := client.Models.GenerateContent(ctx, model, genai.Text(prompt),
			&genai.GenerateContentConfig{ResponseMIMEType: "application/json"})
		if err != nil {
			return "", err
		}
		text := resp.Text()
		if strings.TrimSpace(text) == "" {
			return "", errors.New("gemini returned an empty response")
		}
		return text, nil
	}
	return newGeminiWith(gen, model, defaultGeminiBackoff), nil
}

func newGeminiWith(gen GenerateFunc, model string, backoff []time.Duration) Provider {
	return &gemini{gen: gen, model: model, backoff: backoff}
}

func (g *gemini) Name() string { return "gemini" }

func (g *gemini) Complete(ctx context.Context, prompt string) (string, error) {
	for attempt := 0; ; attempt++ {
		text, err := g.gen(ctx, g.model, prompt)
		if err == nil {
			return text, nil
		}
		if !retryable(err) || attempt >= len(g.backoff) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(g.backoff[attempt]):
		}
	}
}

// retryable reports whether err looks like a rate limit or a transient server error.
func retryable(err error) bool {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429 || apiErr.Code >= 500
	}
	msg := err.Error()
	for _, s := range []string{"429", "500", "503", "RESOURCE_EXHAUSTED", "UNAVAILABLE"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
