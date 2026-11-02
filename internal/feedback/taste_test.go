package feedback

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/store"
)

type fakeTasteStore struct {
	recent, old []store.FeedbackWithItem
	kv          map[string]string
}

func (f *fakeTasteStore) FeedbackSince(time.Time) ([]store.FeedbackWithItem, error) { return f.recent, nil }
func (f *fakeTasteStore) FeedbackBefore(before, after time.Time) ([]store.FeedbackWithItem, error) {
	return f.old, nil
}
func (f *fakeTasteStore) GetKV(k string) (string, bool, error) { v, ok := f.kv[k]; return v, ok, nil }
func (f *fakeTasteStore) SetKV(k, v string) error             { f.kv[k] = v; return nil }

func fb(kind, title, source, text string) store.FeedbackWithItem {
	return store.FeedbackWithItem{Feedback: store.Feedback{Kind: kind, Text: text}, Title: title, Source: source}
}

func TestTasteNotesEmpty(t *testing.T) {
	got, err := TasteNotes(&fakeTasteStore{kv: map[string]string{}}, time.Now(), 14)
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestTasteNotesFormat(t *testing.T) {
	st := &fakeTasteStore{
		kv: map[string]string{"taste_summary": "Prefers Go and CI content."},
		recent: []store.FeedbackWithItem{
			fb("up", "Context hub", "HN", ""),
			fb("down", "AI will replace devs", "r/ClaudeAI", ""),
			fb("comment", "PR skill", "PE", "too basic"),
			fb("comment", "", "", "more Go please"),
		},
	}
	got, _ := TasteNotes(st, time.Now(), 14)
	want := `Owner's standing preferences: Prefers Go and CI content.
Recent feedback (newest first):
- 👍 "Context hub" (HN)
- 👎 "AI will replace devs" (r/ClaudeAI)
- 💬 on "PR skill": too basic
- 💬 general: more Go please`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTasteNotesCapped(t *testing.T) {
	st := &fakeTasteStore{kv: map[string]string{}}
	for i := 0; i < 100; i++ {
		st.recent = append(st.recent, fb("up", "x", "y", ""))
	}
	got, _ := TasteNotes(st, time.Now(), 14)
	if n := strings.Count(got, "\n") + 1; n > 41 {
		t.Fatalf("%d lines", n)
	}
}

func TestRefreshSummarySkipsWhenFresh(t *testing.T) {
	now := time.Now()
	st := &fakeTasteStore{kv: map[string]string{"taste_summary_updated_at": now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)},
		old: []store.FeedbackWithItem{fb("up", "x", "y", "")}}
	called := false
	err := RefreshSummary(context.Background(), st, func(context.Context, string) (string, error) { called = true; return "s", nil }, now, 14)
	if err != nil || called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}

func TestRefreshSummaryFoldsOldFeedback(t *testing.T) {
	now := time.Now()
	st := &fakeTasteStore{kv: map[string]string{"taste_summary": "Likes CI."},
		old: []store.FeedbackWithItem{fb("comment", "Hype post", "HN", "stop sending hype")}}
	var prompt string
	err := RefreshSummary(context.Background(), st, func(_ context.Context, p string) (string, error) {
		prompt = p
		return "  Likes CI. Hates hype.  ", nil
	}, now, 14)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "stop sending hype") || !strings.Contains(prompt, "Likes CI.") || !strings.Contains(prompt, "120 words") {
		t.Errorf("prompt = %s", prompt)
	}
	if st.kv["taste_summary"] != "Likes CI. Hates hype." || st.kv["taste_summary_updated_at"] == "" ||
		st.kv["taste_folded_until"] != now.Add(-14*24*time.Hour).Format(time.RFC3339) {
		t.Errorf("kv = %+v", st.kv)
	}
}

func TestRefreshSummaryNothingOldToFold(t *testing.T) {
	st := &fakeTasteStore{kv: map[string]string{}}
	called := false
	RefreshSummary(context.Background(), st, func(context.Context, string) (string, error) { called = true; return "", nil }, time.Now(), 14)
	if called {
		t.Fatal("provider called with nothing to fold")
	}
}

func TestRefreshSummaryAcceptsJSONProfile(t *testing.T) {
	st := &fakeTasteStore{kv: map[string]string{}, old: []store.FeedbackWithItem{fb("up", "x", "y", "")}}
	RefreshSummary(context.Background(), st, func(context.Context, string) (string, error) {
		return `{"profile": "Wants CI content."}`, nil
	}, time.Now(), 14)
	if st.kv["taste_summary"] != "Wants CI content." {
		t.Fatalf("summary = %q", st.kv["taste_summary"])
	}
}
