package discord

import (
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/store"
)

type fakeFeedbackStore struct {
	digests []store.DigestRecord
	items   map[string][]store.ItemRecord
	got     []store.Feedback
}

func (f *fakeFeedbackStore) RecentDigests(time.Time) ([]store.DigestRecord, error) { return f.digests, nil }
func (f *fakeFeedbackStore) ItemsForDigest(d string) ([]store.ItemRecord, error) { return f.items[d], nil }
func (f *fakeFeedbackStore) UpsertFeedback(fs []store.Feedback) error {
	f.got = append(f.got, fs...)
	return nil
}

func feedbackFixture() (*fakeAPI, *fakeFeedbackStore) {
	posted := time.Now().Add(-24 * time.Hour)
	st := &fakeFeedbackStore{
		digests: []store.DigestRecord{
			{Date: "2026-10-31", ThreadID: "th1", PostedAt: posted},
			{Date: "2026-10-30", ThreadID: "", PostedAt: posted}, // zero-item day: no thread
		},
		items: map[string][]store.ItemRecord{"2026-10-31": {{MessageID: "i1", DigestDate: "2026-10-31"}, {MessageID: "i2", DigestDate: "2026-10-31"}}},
	}
	api := &fakeAPI{
		reactions: map[string][]string{
			"i1 👍": {"bot", "owner"},
			"i2 👎": {"owner"},
			"i2 👍": {"bot", "someone"},
		},
		messages: map[string][]Message{"th1": {
			{ID: "r1", AuthorID: "owner", Content: "too basic", ReplyToID: "i2", Timestamp: posted.Add(time.Hour)},
			{ID: "r2", AuthorID: "owner", Content: "more Go please", Timestamp: posted.Add(2 * time.Hour)},
			{ID: "r3", AuthorID: "someone", Content: "spam", ReplyToID: "i1"},
		}},
	}
	return api, st
}

func byKey(fs []store.Feedback) map[string]store.Feedback {
	m := map[string]store.Feedback{}
	for _, f := range fs {
		m[f.Key] = f
	}
	return m
}

func TestSyncFeedbackOwnerOnly(t *testing.T) {
	api, st := feedbackFixture()
	if err := SyncFeedback(api, st, "owner", time.Now().Add(-14*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	m := byKey(st.got)
	if len(m) != 4 {
		t.Fatalf("got %d feedback rows: %+v", len(m), st.got)
	}
	if m["react:i1:up"].Kind != "up" || m["react:i2:down"].Kind != "down" || m["react:i1:up"].DigestDate != "2026-10-31" {
		t.Errorf("reactions = %+v", m)
	}
	if _, ok := m["react:i2:up"]; ok {
		t.Error("non-owner reaction stored")
	}
	if _, ok := m["reply:r3"]; ok {
		t.Error("non-owner reply stored")
	}
}

func TestSyncFeedbackReplyMapsToItem(t *testing.T) {
	api, st := feedbackFixture()
	SyncFeedback(api, st, "owner", time.Now().Add(-14*24*time.Hour))
	r := byKey(st.got)["reply:r1"]
	if r.Kind != "comment" || r.ItemMessageID != "i2" || r.Text != "too basic" {
		t.Fatalf("reply = %+v", r)
	}
}

func TestSyncFeedbackThreadLevelComment(t *testing.T) {
	api, st := feedbackFixture()
	SyncFeedback(api, st, "owner", time.Now().Add(-14*24*time.Hour))
	r := byKey(st.got)["reply:r2"]
	if r.Kind != "comment" || r.ItemMessageID != "" || r.Text != "more Go please" {
		t.Fatalf("reply = %+v", r)
	}
}
