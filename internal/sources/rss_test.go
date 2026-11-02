package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
	"github.com/manhhung2111/news-pulling-bot/internal/model"
)

func atomFeed(now time.Time) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Blog</title>
  <entry>
    <title>Fresh post</title>
    <link href="https://example.com/fresh?utm_source=rss"/>
    <id>1</id>
    <published>%s</published>
    <summary type="html">&lt;p&gt;Packing a &lt;b&gt;workflow&lt;/b&gt; into a skill &amp;amp; more&lt;/p&gt;</summary>
  </entry>
  <entry>
    <title>Old post</title>
    <link href="https://example.com/old"/>
    <id>2</id>
    <published>%s</published>
  </entry>
  <entry>
    <title>Undated post</title>
    <link href="https://example.com/undated"/>
    <id>3</id>
  </entry>
</feed>`, now.Add(-time.Hour).Format(time.RFC3339), now.Add(-72*time.Hour).Format(time.RFC3339))
}

func feedServer(t *testing.T, wantUA string) *httptest.Server {
	t.Helper()
	body := atomFeed(time.Now())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantUA != "" && r.UserAgent() != wantUA {
			t.Errorf("User-Agent = %q, want %q", r.UserAgent(), wantUA)
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRSSFiltersBySinceAndUndated(t *testing.T) {
	srv := feedServer(t, "")
	src := NewRSS(config.SourceConfig{Name: "Blog", Type: "rss", URL: srv.URL}, Deps{HTTP: srv.Client(), UserAgent: "ua"})
	got, err := src.Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
	}
	c := got[0]
	if c.Title != "Fresh post" || c.Source != "Blog" || c.URL != "https://example.com/fresh?utm_source=rss" {
		t.Errorf("candidate = %+v", c)
	}
	if c.NormURL != "https://example.com/fresh" {
		t.Errorf("NormURL = %q", c.NormURL)
	}
	if c.Excerpt != "Packing a workflow into a skill & more" {
		t.Errorf("Excerpt = %q", c.Excerpt)
	}
}

func TestRSSSetsPaywalled(t *testing.T) {
	srv := feedServer(t, "")
	src := NewRSS(config.SourceConfig{Name: "PE", URL: srv.URL, Paywalled: true}, Deps{HTTP: srv.Client()})
	got, err := src.Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil || len(got) != 1 || !got[0].Paywalled {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestRSSSendsUserAgent(t *testing.T) {
	srv := feedServer(t, "ai-eng-digest/test")
	src := NewRSS(config.SourceConfig{Name: "Blog", URL: srv.URL}, Deps{HTTP: srv.Client(), UserAgent: "ai-eng-digest/test"})
	if _, err := src.Fetch(context.Background(), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

type fakeSource struct {
	name string
	out  []model.Candidate
	err  error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(context.Context, time.Time) ([]model.Candidate, error) {
	return f.out, f.err
}

func TestCollectAllIsolatesFailures(t *testing.T) {
	srcs := []Source{
		fakeSource{name: "A", err: errors.New("403 blocked")},
		fakeSource{name: "B", out: []model.Candidate{{Title: "1"}, {Title: "2"}}},
	}
	cands, statuses := CollectAll(context.Background(), srcs, time.Now())
	if len(cands) != 2 {
		t.Fatalf("got %d candidates", len(cands))
	}
	if len(statuses) != 2 || statuses[0].Name != "A" || statuses[0].OK || !strings.Contains(statuses[0].Err, "403") ||
		statuses[1] != (model.SourceStatus{Name: "B", OK: true, Count: 2}) {
		t.Fatalf("statuses = %+v", statuses)
	}
}
