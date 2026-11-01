package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/ai"
	"github.com/mhung2111/ai-eng-digest/internal/config"
	"github.com/mhung2111/ai-eng-digest/internal/discord"
	"github.com/mhung2111/ai-eng-digest/internal/feedback"
	"github.com/mhung2111/ai-eng-digest/internal/model"
	"github.com/mhung2111/ai-eng-digest/internal/sources"
	"github.com/mhung2111/ai-eng-digest/internal/store"
)

// maxOffered caps how many candidates go to the shortlist prompt (newest first).
const maxOffered = 150

type TLDRFetcher interface {
	FetchTLDR(ctx context.Context, permalink string) (string, error)
}

type Deps struct {
	Cfg     *config.Config
	Store   *store.Store
	Sources []sources.Source
	TLDR    TLDRFetcher // nil when there is no reddit source
	AI      ai.Chain
	Discord discord.API // nil in dry-run
	Article func(ctx context.Context, url string) (string, error)
	Now     func() time.Time
	Out     io.Writer // dry-run output
	Log     *slog.Logger
}

// Summary is what one run did, for the log line.
type Summary struct {
	PerSource map[string]int
	Offered   int
	Picked    int
	Provider  string
}

// Run builds and posts (or, in dry-run, prints) one digest.
func Run(ctx context.Context, d Deps, dryRun bool) (Summary, error) {
	sum, err := run(ctx, d, dryRun)
	var quiet quietError
	if err != nil && !dryRun && d.Discord != nil && !errors.As(err, &quiet) {
		if perr := discord.PostFailure(d.Discord, d.Cfg.Discord.ChannelID, err.Error()); perr != nil {
			d.Log.Error("post failure message failed", "err", perr)
		}
	}
	return sum, err
}

func run(ctx context.Context, d Deps, dryRun bool) (Summary, error) {
	now := d.Now()
	sum := Summary{PerSource: map[string]int{}}
	since := now.Add(-time.Duration(d.Cfg.LookbackHours) * time.Hour)

	collected, statuses := sources.CollectAll(ctx, d.Sources, since)
	var unavailable []string
	for _, s := range statuses {
		sum.PerSource[s.Name] = s.Count
		if !s.OK {
			unavailable = append(unavailable, s.Name)
			d.Log.Warn("source unavailable", "source", s.Name, "err", s.Err)
		}
	}
	cands, err := FilterNew(MergeCandidates(collected), d.Store)
	if err != nil {
		return sum, fmt.Errorf("filter candidates: %w", err)
	}
	if len(cands) > maxOffered {
		cands = cands[:maxOffered]
	}
	AssignIDs(cands)
	sum.Offered = len(cands)

	complete := func(ctx context.Context, p string) (string, error) {
		text, _, err := d.AI.Complete(ctx, p)
		return text, err
	}
	if !dryRun && d.Discord != nil {
		feedbackSince := now.Add(-time.Duration(d.Cfg.FeedbackDays) * 24 * time.Hour)
		if err := discord.SyncFeedback(d.Discord, d.Store, d.Cfg.Discord.OwnerUserID, feedbackSince); err != nil {
			d.Log.Warn("sync feedback failed", "err", err)
		}
		if err := feedback.RefreshSummary(ctx, d.Store, complete, now, d.Cfg.FeedbackDays); err != nil {
			d.Log.Warn("refresh taste summary failed", "err", err)
		}
	}
	taste, err := feedback.TasteNotes(d.Store, now, d.Cfg.FeedbackDays)
	if err != nil {
		d.Log.Warn("taste notes failed", "err", err)
	}

	digest := model.Digest{Date: now.In(d.Cfg.Location()), Unavailable: unavailable}
	if len(cands) > 0 {
		items, provider, err := pick(ctx, d, cands, taste)
		if err != nil {
			return sum, err
		}
		digest.Items, digest.Provider = items, provider
	}
	sum.Picked, sum.Provider = len(digest.Items), digest.Provider
	rendered := discord.Render(digest)

	if dryRun {
		printDigest(d.Out, rendered)
		return sum, nil
	}
	if d.Discord == nil {
		return sum, errors.New("no Discord client configured")
	}
	posted, postErr := discord.PostDigest(d.Discord, d.Cfg.Discord.ChannelID, rendered)
	if posted.MessageID == "" {
		return sum, fmt.Errorf("post digest: %w", postErr)
	}
	// The headline is visible, so its links count as sent even if a later step failed.
	recErr := record(d, digest, cands, posted, now)
	if postErr != nil {
		if recErr != nil {
			d.Log.Error("record after partial post failed", "err", recErr)
		}
		return sum, fmt.Errorf("post digest: %w", postErr)
	}
	if recErr != nil {
		return sum, quietError{recErr} // the digest is posted; don't announce a failure under it
	}
	return sum, nil
}

// quietError is a run error that should be logged but not announced in the channel.
type quietError struct{ error }

func (q quietError) Unwrap() error { return q.error }

// pick runs shortlist → enrich → write → validate and returns the items and the provider that wrote them.
func pick(ctx context.Context, d Deps, cands []model.Candidate, taste string) ([]model.Item, string, error) {
	byID := map[string]model.Candidate{}
	valid := map[string]bool{}
	for _, c := range cands {
		byID[c.ID], valid[c.ID] = c, true
	}
	var ids []string
	provider, err := d.AI.CompleteParsed(ctx, ai.BuildSelectPrompt(cands, taste, d.Cfg.ShortlistSize), func(raw string) error {
		var perr error
		ids, perr = ai.ParseShortlist(raw, valid)
		return perr
	})
	if err != nil {
		return nil, "", fmt.Errorf("shortlist: %w", err)
	}
	if len(ids) == 0 {
		return nil, provider, nil
	}
	shortlist := make([]model.Candidate, 0, len(ids))
	for _, id := range ids {
		c := byID[id]
		if d.Article != nil {
			if text, err := d.Article(ctx, c.URL); err == nil {
				c.ArticleText = text
			} else {
				d.Log.Info("article fetch failed", "url", c.URL, "err", err)
			}
		}
		if d.TLDR != nil && c.RedditPermalink != "" {
			if tldr, err := d.TLDR.FetchTLDR(ctx, c.RedditPermalink); err == nil {
				c.CommunityTLDR = tldr
			} else {
				d.Log.Info("reddit TL;DR fetch failed", "url", c.RedditPermalink, "err", err)
			}
		}
		byID[id] = c
		shortlist = append(shortlist, c)
	}
	var items []model.Item
	provider, err = d.AI.CompleteParsed(ctx, ai.BuildWritePrompt(shortlist, taste, d.Cfg.MaxItems), func(raw string) error {
		var perr error
		items, perr = ai.ParseItems(raw)
		return perr
	})
	if err != nil {
		return nil, "", fmt.Errorf("write: %w", err)
	}
	return ai.ValidateItems(items, byID, d.Cfg.MaxItems), provider, nil
}

func record(d Deps, digest model.Digest, offered []model.Candidate, posted discord.Posted, now time.Time) error {
	date := digest.Date.Format("2006-01-02")
	normOf := map[string]string{}
	var seen []string
	for _, c := range offered {
		normOf[c.ID] = c.NormURL
		seen = append(seen, c.NormURL)
	}
	var items []store.ItemRecord
	for i, it := range digest.Items {
		msgID := fmt.Sprintf("unposted:%s:%d", date, i+1) // listed in the headline, but its thread message never went out
		if i < len(posted.ItemMessageIDs) {
			msgID = posted.ItemMessageIDs[i]
		}
		items = append(items, store.ItemRecord{MessageID: msgID, DigestDate: date,
			NormURL: normOf[it.CandidateID], Title: it.Title, Source: it.Source})
	}
	rec := store.DigestRecord{Date: date, ChannelID: d.Cfg.Discord.ChannelID, MessageID: posted.MessageID,
		ThreadID: posted.ThreadID, Provider: digest.Provider, PostedAt: now}
	if err := d.Store.RecordDigest(rec, items); err != nil {
		return fmt.Errorf("record digest: %w", err)
	}
	if err := d.Store.MarkSeen(seen, now); err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
}

func printDigest(w io.Writer, r discord.Rendered) {
	fmt.Fprintln(w, r.Headline)
	for _, e := range r.Items {
		fmt.Fprintf(w, "\n──────── %s\n%s\n%s\n", e.Title, e.URL, e.Description)
		for _, f := range e.Fields {
			fmt.Fprintf(w, "\n%s\n%s\n", f.Name, f.Value)
		}
	}
}
