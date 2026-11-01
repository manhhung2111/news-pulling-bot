package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProvider struct {
	name  string
	out   string
	err   error
	calls int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Complete(context.Context, string) (string, error) {
	f.calls++
	return f.out, f.err
}

func TestChainFallsBack(t *testing.T) {
	a := &fakeProvider{name: "claude", err: errors.New("limit")}
	b := &fakeProvider{name: "gemini", out: "ok"}
	text, prov, err := Chain{Providers: []Provider{a, b}}.Complete(context.Background(), "p")
	if err != nil || text != "ok" || prov != "gemini" {
		t.Fatalf("got %q %q %v", text, prov, err)
	}
}

func TestChainStopsAtFirstSuccess(t *testing.T) {
	a := &fakeProvider{name: "claude", out: "ok"}
	b := &fakeProvider{name: "gemini", out: "no"}
	_, prov, _ := Chain{Providers: []Provider{a, b}}.Complete(context.Background(), "p")
	if prov != "claude" || b.calls != 0 {
		t.Fatalf("prov=%s gemini calls=%d", prov, b.calls)
	}
}

func TestChainAllFail(t *testing.T) {
	a := &fakeProvider{name: "claude", err: errors.New("limit")}
	b := &fakeProvider{name: "gemini", err: errors.New("429")}
	_, _, err := Chain{Providers: []Provider{a, b}}.Complete(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "gemini") {
		t.Fatalf("err = %v", err)
	}
}
