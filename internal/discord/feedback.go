package discord

import (
	"log/slog"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/store"
)

type FeedbackStore interface {
	RecentDigests(since time.Time) ([]store.DigestRecord, error)
	ItemsForDigest(date string) ([]store.ItemRecord, error)
	UpsertFeedback([]store.Feedback) error
}

var reactionKinds = map[string]string{"👍": "up", "👎": "down"}

// SyncFeedback reads the owner's reactions and replies from recent digest threads into the store.
// Other users' feedback is ignored; a digest whose thread can't be read is skipped.
func SyncFeedback(api API, st FeedbackStore, ownerID string, since time.Time) error {
	digests, err := st.RecentDigests(since)
	if err != nil {
		return err
	}
	for _, d := range digests {
		if d.ThreadID == "" {
			continue
		}
		items, err := st.ItemsForDigest(d.Date)
		if err != nil {
			return err
		}
		isItem := map[string]bool{}
		var fs []store.Feedback
		for _, it := range items {
			isItem[it.MessageID] = true
			for emoji, kind := range reactionKinds {
				users, err := api.Reactions(d.ThreadID, it.MessageID, emoji)
				if err != nil {
					slog.Warn("read reactions failed", "digest", d.Date, "err", err)
					continue
				}
				if contains(users, ownerID) {
					fs = append(fs, store.Feedback{Key: "react:" + it.MessageID + ":" + kind, DigestDate: d.Date,
						ItemMessageID: it.MessageID, Kind: kind, CreatedAt: d.PostedAt})
				}
			}
		}
		msgs, err := api.Messages(d.ThreadID, 100)
		if err != nil {
			slog.Warn("read thread failed", "digest", d.Date, "err", err)
		}
		for _, m := range msgs {
			if m.AuthorID != ownerID || m.Content == "" {
				continue
			}
			f := store.Feedback{Key: "reply:" + m.ID, DigestDate: d.Date, Kind: "comment", Text: m.Content, CreatedAt: m.Timestamp}
			if isItem[m.ReplyToID] {
				f.ItemMessageID = m.ReplyToID
			}
			fs = append(fs, f)
		}
		if err := st.UpsertFeedback(fs); err != nil {
			return err
		}
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
