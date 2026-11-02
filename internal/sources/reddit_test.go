package sources

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
)

func redditListing(now time.Time) string {
	ts := now.Add(-3 * time.Hour).Format(time.RFC3339)
	linkContent := html.EscapeString(`<table><tr><td> submitted by <a href="https://www.reddit.com/user/alice"> /u/alice </a> <br/> <span><a href="https://blog.example.com/context-hub?a=1&amp;b=2">[link]</a></span> <span><a href="https://www.reddit.com/r/ClaudeAI/comments/aaa/context_hub/">[comments]</a></span></td></tr></table>`)
	selfContent := html.EscapeString(`<div class="md"><p>I packed my PR review workflow into a skill.</p></div> submitted by <a href="https://www.reddit.com/user/bob"> /u/bob </a> <span><a href="https://www.reddit.com/r/ClaudeAI/comments/bbb/pr_skill/">[link]</a></span> <span><a href="https://www.reddit.com/r/ClaudeAI/comments/bbb/pr_skill/">[comments]</a></span>`)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><author><name>/u/alice</name></author><content type="html">%s</content><id>t3_aaa</id>
<link href="https://www.reddit.com/r/ClaudeAI/comments/aaa/context_hub/" /><published>%s</published><title>A context hub for agents</title></entry>
<entry><author><name>/u/bob</name></author><content type="html">%s</content><id>t3_bbb</id>
<link href="https://www.reddit.com/r/ClaudeAI/comments/bbb/pr_skill/" /><published>%s</published><title>PR review as a skill</title></entry>
</feed>`, linkContent, ts, selfContent, ts)
}

func redditComments(botAuthor, botText string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><author><name>/u/bob</name></author><content type="html">%s</content><id>t3_bbb</id><title>PR review as a skill</title></entry>
<entry><author><name>/u/carol</name></author><content type="html">%s</content><id>t1_c1</id><title>/u/carol on PR review as a skill</title></entry>
<entry><author><name>/u/%s</name></author><content type="html">%s</content><id>t1_c2</id><title>bot comment</title></entry>
</feed>`, html.EscapeString("<p>post body TL;DR inside post</p>"), html.EscapeString("<p>Nice one</p>"), botAuthor, html.EscapeString(botText))
}

type redditFixture struct {
	listingStatus int
	comments      string
	gotPaths      []string
}

func (f *redditFixture) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.gotPaths = append(f.gotPaths, r.URL.RequestURI())
		switch {
		case strings.HasPrefix(r.URL.Path, "/r/ClaudeAI/top/"):
			if f.listingStatus != 0 {
				w.WriteHeader(f.listingStatus)
				return
			}
			fmt.Fprint(w, redditListing(time.Now()))
		case strings.HasSuffix(r.URL.Path, "/.rss"):
			fmt.Fprint(w, f.comments)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var redditCfg = config.SourceConfig{Name: "r/ClaudeAI", Type: "reddit_rss", Subreddit: "ClaudeAI", Listing: "top", Period: "day"}

func TestRedditLinkPostUsesExternalURL(t *testing.T) {
	f := &redditFixture{}
	srv := f.server(t)
	got, err := NewReddit(redditCfg, Deps{HTTP: srv.Client()}, srv.URL).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	c := got[0]
	if c.URL != "https://blog.example.com/context-hub?a=1&b=2" || c.NormURL != "https://blog.example.com/context-hub?a=1&b=2" || len(c.Discussion) != 1 ||
		c.Discussion[0] != "https://www.reddit.com/r/ClaudeAI/comments/aaa/context_hub/" ||
		c.RedditPermalink != "https://www.reddit.com/r/ClaudeAI/comments/aaa/context_hub/" || c.Source != "r/ClaudeAI" {
		t.Errorf("link post = %+v", c)
	}
	if f.gotPaths[0] != "/r/ClaudeAI/top/.rss?t=day" {
		t.Errorf("listing path = %q", f.gotPaths[0])
	}
}

func TestRedditSelfPostUsesPermalink(t *testing.T) {
	f := &redditFixture{}
	srv := f.server(t)
	got, _ := NewReddit(redditCfg, Deps{HTTP: srv.Client()}, srv.URL).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	c := got[1]
	if c.URL != "https://www.reddit.com/r/ClaudeAI/comments/bbb/pr_skill/" || len(c.Discussion) != 0 ||
		c.RedditPermalink != c.URL || !strings.Contains(c.Excerpt, "PR review workflow") {
		t.Errorf("self post = %+v", c)
	}
}

func TestRedditBlockedReturnsError(t *testing.T) {
	f := &redditFixture{listingStatus: 403}
	srv := f.server(t)
	_, err := NewReddit(redditCfg, Deps{HTTP: srv.Client()}, srv.URL).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
}

const permalink = "https://www.reddit.com/r/ClaudeAI/comments/bbb/pr_skill/"

func TestRedditTLDRByAuthor(t *testing.T) {
	f := &redditFixture{comments: redditComments("ClaudeAI-mod-bot", "<p>Summary: people liked it, but slow on big diffs.</p>")}
	srv := f.server(t)
	cfg := redditCfg
	cfg.TLDRAuthor = "ClaudeAI-mod-bot"
	got, err := NewReddit(cfg, Deps{HTTP: srv.Client()}, srv.URL).FetchTLDR(context.Background(), permalink)
	if err != nil || got != "Summary: people liked it, but slow on big diffs." {
		t.Fatalf("got %q, %v", got, err)
	}
	if f.gotPaths[0] != "/r/ClaudeAI/comments/bbb/pr_skill/.rss" {
		t.Errorf("comments path = %q", f.gotPaths[0])
	}
}

func TestRedditTLDRByMarker(t *testing.T) {
	f := &redditFixture{comments: redditComments("somebot", "<p><strong>TL;DR of the discussion:</strong> mostly positive.</p>")}
	srv := f.server(t)
	cfg := redditCfg
	cfg.TLDRMarker = "TL;DR of the discussion"
	got, err := NewReddit(cfg, Deps{HTTP: srv.Client()}, srv.URL).FetchTLDR(context.Background(), permalink)
	if err != nil || got != "TL;DR of the discussion: mostly positive." {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRedditTLDRNoneFound(t *testing.T) {
	f := &redditFixture{comments: redditComments("someone", "<p>just a comment</p>")}
	srv := f.server(t)
	cfg := redditCfg
	cfg.TLDRAuthor = "ClaudeAI-mod-bot"
	cfg.TLDRMarker = "TL;DR of the discussion"
	got, err := NewReddit(cfg, Deps{HTTP: srv.Client()}, srv.URL).FetchTLDR(context.Background(), permalink)
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRedditSelfPostCarriesBodyAsArticleText(t *testing.T) {
	f := &redditFixture{}
	srv := f.server(t)
	got, _ := NewReddit(redditCfg, Deps{HTTP: srv.Client()}, srv.URL).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if !strings.Contains(got[1].ArticleText, "PR review workflow") {
		t.Errorf("self post ArticleText = %q", got[1].ArticleText)
	}
	if got[0].ArticleText != "" {
		t.Errorf("link post ArticleText = %q, want empty so the linked article is fetched", got[0].ArticleText)
	}
}

func TestRedditTLDRUnconfiguredMakesNoRequest(t *testing.T) {
	f := &redditFixture{comments: redditComments("bot", "<p>TL;DR</p>")}
	srv := f.server(t)
	got, err := NewReddit(redditCfg, Deps{HTTP: srv.Client()}, srv.URL).FetchTLDR(context.Background(), permalink)
	if err != nil || got != "" || len(f.gotPaths) != 0 {
		t.Fatalf("got %q, %v, requests %v", got, err, f.gotPaths)
	}
}

func TestRedditTLDRSpacesRequests(t *testing.T) {
	f := &redditFixture{comments: redditComments("bot", "<p>x</p>")}
	srv := f.server(t)
	cfg := redditCfg
	cfg.TLDRAuthor = "bot"
	r := NewReddit(cfg, Deps{HTTP: srv.Client()}, srv.URL)
	r.gap = 200 * time.Millisecond
	start := time.Now()
	for i := 0; i < 2; i++ {
		if _, err := r.FetchTLDR(context.Background(), permalink); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 200*time.Millisecond {
		t.Errorf("two TL;DR fetches took %v, want ≥ 200ms apart", el)
	}
}
