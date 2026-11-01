package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s.Close()
}

func TestRecordDigestMarksSent(t *testing.T) {
	s := openTemp(t)
	err := s.RecordDigest(DigestRecord{Date: "2026-11-01", ChannelID: "c", MessageID: "m", ThreadID: "t", Provider: "claude", PostedAt: time.Now()},
		[]ItemRecord{{MessageID: "i1", DigestDate: "2026-11-01", NormURL: "https://a.com/x", Title: "X", Source: "S"}})
	if err != nil {
		t.Fatal(err)
	}
	sent, err := s.IsSent("https://a.com/x")
	if err != nil || !sent {
		t.Fatalf("IsSent = %v, %v", sent, err)
	}
	items, err := s.ItemsForDigest("2026-11-01")
	if err != nil || len(items) != 1 || items[0].MessageID != "i1" {
		t.Fatalf("ItemsForDigest = %+v, %v", items, err)
	}
}

func TestMarkSeen(t *testing.T) {
	s := openTemp(t)
	u := "https://a.com/y"
	if seen, _ := s.IsSeen(u); seen {
		t.Fatal("seen before marking")
	}
	if err := s.MarkSeen([]string{u}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSeen([]string{u}, time.Now()); err != nil {
		t.Fatalf("second mark: %v", err)
	}
	if seen, _ := s.IsSeen(u); !seen {
		t.Fatal("not seen after marking")
	}
}

func TestUpsertFeedbackDedupesByKey(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	if err := s.RecordDigest(DigestRecord{Date: "2026-11-01", PostedAt: now},
		[]ItemRecord{{MessageID: "i1", DigestDate: "2026-11-01", NormURL: "https://a.com/x", Title: "X", Source: "S"}}); err != nil {
		t.Fatal(err)
	}
	f := Feedback{Key: "react:i1:up", DigestDate: "2026-11-01", ItemMessageID: "i1", Kind: "up", CreatedAt: now}
	if err := s.UpsertFeedback([]Feedback{f, f}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFeedback([]Feedback{f}); err != nil {
		t.Fatal(err)
	}
	got, err := s.FeedbackSince(now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "X" || got[0].Source != "S" {
		t.Fatalf("FeedbackSince = %+v", got)
	}
}

func TestFeedbackBefore(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	old := Feedback{Key: "reply:1", Kind: "comment", Text: "old", CreatedAt: now.Add(-20 * 24 * time.Hour)}
	recent := Feedback{Key: "reply:2", Kind: "comment", Text: "new", CreatedAt: now}
	if err := s.UpsertFeedback([]Feedback{old, recent}); err != nil {
		t.Fatal(err)
	}
	got, err := s.FeedbackBefore(now.Add(-14*24*time.Hour), time.Time{})
	if err != nil || len(got) != 1 || got[0].Text != "old" {
		t.Fatalf("FeedbackBefore = %+v, %v", got, err)
	}
}

func TestRecentDigestsFiltersByTime(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	s.RecordDigest(DigestRecord{Date: "2026-10-01", PostedAt: now.Add(-30 * 24 * time.Hour)}, nil)
	s.RecordDigest(DigestRecord{Date: "2026-11-01", ThreadID: "t", PostedAt: now}, nil)
	got, err := s.RecentDigests(now.Add(-14 * 24 * time.Hour))
	if err != nil || len(got) != 1 || got[0].Date != "2026-11-01" || got[0].ThreadID != "t" {
		t.Fatalf("RecentDigests = %+v, %v", got, err)
	}
}

func TestKV(t *testing.T) {
	s := openTemp(t)
	if _, ok, err := s.GetKV("k"); ok || err != nil {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}
	if err := s.SetKV("k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKV("k", "v2"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.GetKV("k")
	if !ok || err != nil || v != "v2" {
		t.Fatalf("GetKV = %q %v %v", v, ok, err)
	}
}
