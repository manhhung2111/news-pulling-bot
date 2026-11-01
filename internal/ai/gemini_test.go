package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

var fastBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

func scripted(errs ...error) (GenerateFunc, *int) {
	calls := 0
	return func(ctx context.Context, model, prompt string) (string, error) {
		i := calls
		calls++
		if i < len(errs) && errs[i] != nil {
			return "", errs[i]
		}
		return `{"ok":true}`, nil
	}, &calls
}

func TestGeminiRetriesOn429ThenSucceeds(t *testing.T) {
	gen, calls := scripted(errors.New("Error 429, RESOURCE_EXHAUSTED"), errors.New("Error 503: unavailable"))
	got, err := newGeminiWith(gen, "m", fastBackoff).Complete(context.Background(), "p")
	if err != nil || got != `{"ok":true}` || *calls != 3 {
		t.Fatalf("got %q %v calls=%d", got, err, *calls)
	}
}

func TestGeminiGivesUpAfterRetries(t *testing.T) {
	e := errors.New("Error 429")
	gen, calls := scripted(e, e, e, e, e)
	_, err := newGeminiWith(gen, "m", fastBackoff).Complete(context.Background(), "p")
	if err == nil || *calls != 4 {
		t.Fatalf("err=%v calls=%d", err, *calls)
	}
}

func TestGeminiNoRetryOn400(t *testing.T) {
	gen, calls := scripted(errors.New("Error 400: INVALID_ARGUMENT"))
	_, err := newGeminiWith(gen, "m", fastBackoff).Complete(context.Background(), "p")
	if err == nil || *calls != 1 {
		t.Fatalf("err=%v calls=%d", err, *calls)
	}
}

func TestNewGeminiRequiresKey(t *testing.T) {
	if _, err := NewGemini("", "m"); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestGeminiName(t *testing.T) {
	gen, _ := scripted()
	if newGeminiWith(gen, "m", fastBackoff).Name() != "gemini" {
		t.Fatal("name")
	}
}
