// Package feedback turns the owner's reactions and replies into taste notes for the AI.
package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/store"
)

type TasteStore interface {
	FeedbackSince(time.Time) ([]store.FeedbackWithItem, error)
	FeedbackBefore(before, after time.Time) ([]store.FeedbackWithItem, error)
	GetKV(string) (string, bool, error)
	SetKV(string, string) error
}

// CompleteFunc sends a prompt to the AI; the pipeline passes a wrapper around ai.Chain.Complete.
type CompleteFunc func(ctx context.Context, prompt string) (string, error)

const (
	maxFeedbackLines = 40
	kvSummary        = "taste_summary"
	kvUpdatedAt      = "taste_summary_updated_at"
	kvFoldedUntil    = "taste_folded_until"
	refreshEvery     = 7 * 24 * time.Hour
)

func window(now time.Time, days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }

func line(f store.FeedbackWithItem) string {
	switch f.Kind {
	case "up":
		return fmt.Sprintf("- 👍 %q (%s)", f.Title, f.Source)
	case "down":
		return fmt.Sprintf("- 👎 %q (%s)", f.Title, f.Source)
	default:
		if f.Title == "" {
			return "- 💬 general: " + f.Text
		}
		return fmt.Sprintf("- 💬 on %q: %s", f.Title, f.Text)
	}
}

// TasteNotes renders the standing summary plus recent feedback, or "" when there is none.
func TasteNotes(st TasteStore, now time.Time, windowDays int) (string, error) {
	summary, _, err := st.GetKV(kvSummary)
	if err != nil {
		return "", err
	}
	recent, err := st.FeedbackSince(window(now, windowDays))
	if err != nil {
		return "", err
	}
	var lines []string
	if summary != "" {
		lines = append(lines, "Owner's standing preferences: "+summary)
	}
	if len(recent) > 0 {
		lines = append(lines, "Recent feedback (newest first):")
		for _, f := range recent {
			if len(lines) >= maxFeedbackLines {
				break
			}
			lines = append(lines, line(f))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// RefreshSummary folds feedback older than the window into the standing summary, at most once a week.
func RefreshSummary(ctx context.Context, st TasteStore, complete CompleteFunc, now time.Time, windowDays int) error {
	if v, ok, err := st.GetKV(kvUpdatedAt); err != nil {
		return err
	} else if ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil && now.Sub(t) < refreshEvery {
			return nil
		}
	}
	var after time.Time
	if v, ok, _ := st.GetKV(kvFoldedUntil); ok {
		after, _ = time.Parse(time.RFC3339, v)
	}
	cutoff := window(now, windowDays)
	old, err := st.FeedbackBefore(cutoff, after)
	if err != nil {
		return err
	}
	if len(old) == 0 {
		return nil
	}
	summary, _, _ := st.GetKV(kvSummary)
	var b strings.Builder
	b.WriteString("You maintain a short profile of what a software engineer wants from their daily digest about applying AI coding agents across the SDLC.\n")
	fmt.Fprintf(&b, "Current profile: %s\n\nNew feedback to fold in:\n", orNone(summary))
	for _, f := range old {
		b.WriteString(line(f) + "\n")
	}
	b.WriteString("\nWrite the updated profile in at most 120 words, focused on what they want more and less of. " +
		`Reply with JSON only: {"profile": "..."}`)
	text, err := complete(ctx, b.String())
	if err != nil {
		return err
	}
	if err := st.SetKV(kvSummary, profileText(text)); err != nil {
		return err
	}
	if err := st.SetKV(kvUpdatedAt, now.Format(time.RFC3339)); err != nil {
		return err
	}
	return st.SetKV(kvFoldedUntil, cutoff.Format(time.RFC3339))
}

func orNone(s string) string {
	if s == "" {
		return "(none yet)"
	}
	return s
}

// profileText accepts {"profile": "..."} (Gemini returns JSON) or plain text.
func profileText(raw string) string {
	var p struct {
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &p); err == nil && p.Profile != "" {
		return strings.TrimSpace(p.Profile)
	}
	return strings.TrimSpace(raw)
}
