package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/mhung2111/ai-eng-digest/internal/ai"
	"github.com/mhung2111/ai-eng-digest/internal/config"
	"github.com/mhung2111/ai-eng-digest/internal/discord"
	"github.com/mhung2111/ai-eng-digest/internal/model"
	"github.com/mhung2111/ai-eng-digest/internal/sources"
	"github.com/mhung2111/ai-eng-digest/internal/store"
	"github.com/mhung2111/ai-eng-digest/internal/urlnorm"
)

type stubSource struct {
	name string
	out  []model.Candidate
	err  error
}

func (s stubSource) Name() string { return s.name }
func (s stubSource) Fetch(context.Context, time.Time) ([]model.Candidate, error) {
	return s.out, s.err
}

// scriptedAI answers select prompts with selectOut and write prompts with writeOut.
type scriptedAI struct {
	name      string
	err       error
	selectOut string
	writeOut  string
	prompts   []string
}

func (s *scriptedAI) Name() string { return s.name }
func (s *scriptedAI) Complete(_ context.Context, p string) (string, error) {
	s.prompts = append(s.prompts, p)
	if s.err != nil {
		return "", s.err
	}
	if strings.Contains(p, "## Shortlisted candidates") {
		return s.writeOut, nil
	}
	return s.selectOut, nil
}

type recAPI struct {
	threadErr error
	sent      []string
	embeds    []string
	n         int
}

func (r *recAPI) next() string { r.n++; return fmt.Sprintf("m%d", r.n) }
func (r *recAPI) Send(ch, content string) (string, error) {
	r.sent = append(r.sent, content)
	return r.next(), nil
}
func (r *recAPI) StartThread(ch, msg, name string) (string, error) { return "th", r.threadErr }
func (r *recAPI) SendEmbed(ch string, e *discordgo.MessageEmbed) (string, error) {
	r.embeds = append(r.embeds, e.Title)
	return r.next(), nil
}
func (r *recAPI) React(string, string, string) error                 { return nil }
func (r *recAPI) Reactions(string, string, string) ([]string, error) { return nil, nil }
func (r *recAPI) Messages(string, int) ([]discord.Message, error)    { return nil, nil }

func c(src, url, title string) model.Candidate {
	return model.Candidate{Title: title, URL: url, NormURL: urlnorm.Normalize(url), Source: src, Published: time.Now().Add(-time.Hour)}
}

func older(x model.Candidate) model.Candidate { x.Published = x.Published.Add(-time.Hour); return x }

const writeJSON = `{"items":[{"candidate_id":"c1","title":"Context hub","hook":"Give agents memory","summary":"Build a hub [1].",
"try_this":["Create docs/decisions.md"],"why_it_works":"Less re-explaining [1].","problem_solved":"Agents forget [1].",
"citations":[{"n":1,"url":"https://a.com/hub","label":"article"}]}]}`

type harness struct {
	deps Deps
	st   *store.Store
	api  *recAPI
	ai   *scriptedAI
	out  *bytes.Buffer
}

func newHarness(t *testing.T, srcs ...sources.Source) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if srcs == nil {
		srcs = []sources.Source{stubSource{name: "Blog", out: []model.Candidate{
			c("Blog", "https://a.com/hub", "Context hub"), older(c("Blog", "https://b.com/hype", "AI hype")),
		}}}
	}
	cfg := &config.Config{LookbackHours: 48, MaxItems: 5, ShortlistSize: 8, FeedbackDays: 14, Timezone: "Asia/Ho_Chi_Minh",
		Discord: config.DiscordConfig{ChannelID: "chan", OwnerUserID: "owner"}}
	h := &harness{st: st, api: &recAPI{}, out: &bytes.Buffer{},
		ai: &scriptedAI{name: "claude", selectOut: `{"ids":["c1"]}`, writeOut: writeJSON}}
	h.deps = Deps{
		Cfg: cfg, Store: st, Sources: srcs, AI: ai.Chain{Providers: []ai.Provider{h.ai}}, Discord: h.api,
		Article: func(context.Context, string) (string, error) { return "ARTICLE BODY", nil },
		Now:     time.Now, Out: h.out, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return h
}

func TestRunHappyPath(t *testing.T) {
	h := newHarness(t)
	sum, err := Run(context.Background(), h.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.api.sent) != 1 || !strings.Contains(h.api.sent[0], "1 pick") || len(h.api.embeds) != 1 {
		t.Fatalf("sent=%v embeds=%v", h.api.sent, h.api.embeds)
	}
	if sent, _ := h.st.IsSent("https://a.com/hub"); !sent {
		t.Error("picked URL not marked sent")
	}
	for _, u := range []string{"https://a.com/hub", "https://b.com/hype"} {
		if seen, _ := h.st.IsSeen(u); !seen {
			t.Errorf("%s not marked seen", u)
		}
	}
	if !strings.Contains(h.ai.prompts[1], "ARTICLE BODY") {
		t.Error("write prompt missing article text")
	}
	if sum.Picked != 1 || sum.Provider != "claude" || sum.PerSource["Blog"] != 2 {
		t.Errorf("summary = %+v", sum)
	}
}

func TestRunDryRunWritesOutputOnly(t *testing.T) {
	h := newHarness(t)
	h.deps.Discord = nil
	if _, err := Run(context.Background(), h.deps, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.out.String(), "1 pick") || !strings.Contains(h.out.String(), "Create docs/decisions.md") {
		t.Fatalf("out = %s", h.out.String())
	}
	if seen, _ := h.st.IsSeen("https://a.com/hub"); seen {
		t.Error("dry run marked seen")
	}
}

func TestRunNoCandidatesPostsNothingMetTheBar(t *testing.T) {
	h := newHarness(t, stubSource{name: "Blog"})
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if len(h.ai.prompts) != 0 {
		t.Error("AI called with no candidates")
	}
	if len(h.api.sent) != 1 || !strings.Contains(h.api.sent[0], "Nothing met the bar today.") {
		t.Fatalf("sent = %v", h.api.sent)
	}
}

func TestRunAIFailurePostsFailureAndDoesNotMarkSeen(t *testing.T) {
	h := newHarness(t)
	h.ai.err = errors.New("usage limit")
	if _, err := Run(context.Background(), h.deps, false); err == nil {
		t.Fatal("expected error")
	}
	if len(h.api.sent) != 1 || !strings.HasPrefix(h.api.sent[0], "❌ Digest failed today:") {
		t.Fatalf("sent = %v", h.api.sent)
	}
	if seen, _ := h.st.IsSeen("https://a.com/hub"); seen {
		t.Error("failed run marked seen")
	}
}

func TestRunUnavailableSourceNoted(t *testing.T) {
	h := newHarness(t, stubSource{name: "Blog", out: []model.Candidate{c("Blog", "https://a.com/hub", "Context hub")}},
		stubSource{name: "r/ClaudeAI", err: errors.New("403")})
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.api.sent[0], "⚠️ r/ClaudeAI unavailable today") {
		t.Fatalf("headline = %s", h.api.sent[0])
	}
}

func TestRunUsesFallbackProviderName(t *testing.T) {
	h := newHarness(t)
	failing := &scriptedAI{name: "claude", err: errors.New("limit")}
	h.ai.name = "gemini"
	h.deps.AI = ai.Chain{Providers: []ai.Provider{failing, h.ai}}
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.api.sent[0], "_Made with fallback model (Gemini)._") {
		t.Fatalf("headline = %s", h.api.sent[0])
	}
}

func TestRunRetriesWriteOnBadJSON(t *testing.T) {
	h := newHarness(t)
	calls := 0
	h.deps.AI = ai.Chain{Providers: []ai.Provider{providerFunc(func(p string) (string, error) {
		if !strings.Contains(p, "## Shortlisted candidates") {
			return `{"ids":["c1"]}`, nil
		}
		calls++
		if calls == 1 {
			return "sorry, not json", nil
		}
		return writeJSON, nil
	})}}
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(h.api.embeds) != 1 {
		t.Fatalf("write calls=%d embeds=%d", calls, len(h.api.embeds))
	}
}

type providerFunc func(string) (string, error)

func (f providerFunc) Name() string                                         { return "claude" }
func (f providerFunc) Complete(_ context.Context, p string) (string, error) { return f(p) }

func TestRunSecondDayDoesNotReofferSeen(t *testing.T) {
	h := newHarness(t)
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	prompts := len(h.ai.prompts)
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if len(h.ai.prompts) != prompts {
		t.Error("second run called the AI with already-seen candidates")
	}
	if !strings.Contains(h.api.sent[1], "Nothing met the bar today.") {
		t.Fatalf("second headline = %s", h.api.sent[1])
	}
}

func TestRunThreadFailureStillRecordsSentAndSeen(t *testing.T) {
	h := newHarness(t)
	h.api.threadErr = errors.New("missing Create Public Threads permission")
	if _, err := Run(context.Background(), h.deps, false); err == nil {
		t.Fatal("expected error")
	}
	if sent, _ := h.st.IsSent("https://a.com/hub"); !sent {
		t.Error("item shown in headline was not recorded as sent")
	}
	if seen, _ := h.st.IsSeen("https://b.com/hype"); !seen {
		t.Error("offered candidates not marked seen after headline was posted")
	}
	if len(h.api.sent) != 2 || !strings.Contains(h.api.sent[1], "start thread") {
		t.Fatalf("sent = %v", h.api.sent)
	}
}

func TestRunDropsItemsNotShortlisted(t *testing.T) {
	h := newHarness(t)
	// Shortlist only c1, but the writer returns c2 (never read in full).
	h.ai.writeOut = strings.ReplaceAll(strings.ReplaceAll(writeJSON, `"c1"`, `"c2"`), "https://a.com/hub", "https://b.com/hype")
	if _, err := Run(context.Background(), h.deps, false); err != nil {
		t.Fatal(err)
	}
	if len(h.api.embeds) != 0 || !strings.Contains(h.api.sent[0], "Nothing met the bar today.") {
		t.Fatalf("non-shortlisted item was posted: %v", h.api.sent)
	}
}
