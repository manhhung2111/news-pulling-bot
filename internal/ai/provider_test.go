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

func TestCompleteParsedFallsBackOnBadJSON(t *testing.T) {
	a := &fakeProvider{name: "claude", out: "not json"}
	b := &fakeProvider{name: "gemini", out: `{"ok":true}`}
	var got string
	prov, err := Chain{Providers: []Provider{a, b}}.CompleteParsed(context.Background(), "p", func(raw string) error {
		if !strings.HasPrefix(raw, "{") {
			return errors.New("bad json")
		}
		got = raw
		return nil
	})
	if err != nil || prov != "gemini" || got != `{"ok":true}` || a.calls != 2 {
		t.Fatalf("prov=%q got=%q err=%v claude calls=%d", prov, got, err, a.calls)
	}
}

func TestCompleteParsedAllBad(t *testing.T) {
	a := &fakeProvider{name: "claude", out: "x"}
	_, err := Chain{Providers: []Provider{a}}.CompleteParsed(context.Background(), "p", func(string) error { return errors.New("bad json") })
	if err == nil || !strings.Contains(err.Error(), "bad json") {
		t.Fatalf("err = %v", err)
	}
}
