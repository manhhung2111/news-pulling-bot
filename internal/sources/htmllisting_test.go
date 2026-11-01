package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/config"
)

type listingFixture struct {
	mu   sync.Mutex
	hits map[string]int
}

func (f *listingFixture) server(t *testing.T) *httptest.Server {
	f.hits = map[string]int{}
	recent := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	old := time.Now().Add(-5 * 24 * time.Hour).Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		f.mu.Unlock()
		switch r.URL.Path {
		case "/news":
			fmt.Fprint(w, `<html><body>
			  <a href="/news/context-engineering">Context engineering</a>
			  <a href="/news/context-engineering"><img/></a>
			  <a href="/careers">Careers</a>
			  <a href="/news/">All news</a>
			  <a href="/news/old-post">Old</a>
			  <a href="/news/broken">Broken</a>
			</body></html>`)
		case "/news/context-engineering":
			fmt.Fprintf(w, `<html><head><meta property="og:title" content="Effective context engineering"/>
			  <meta property="article:published_time" content="%s"/><meta name="description" content="Treat context as a budget."/>
			  <title>ignored</title></head><body></body></html>`, recent)
		case "/news/old-post":
			fmt.Fprintf(w, `<html><head><title>Old post</title></head><body><time datetime="%s">Old</time></body></html>`, old)
		case "/news/broken":
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTMLListingKeepsRecentPrefixedLinks(t *testing.T) {
	f := &listingFixture{}
	srv := f.server(t)
	cfg := config.SourceConfig{Name: "Anthropic News", Type: "html_listing", URL: srv.URL + "/news", LinkPrefix: "/news/"}
	got, err := NewHTMLListing(cfg, Deps{HTTP: srv.Client()}).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Title != "Effective context engineering" || c.URL != srv.URL+"/news/context-engineering" ||
		c.Excerpt != "Treat context as a budget." || c.Source != "Anthropic News" || c.NormURL == "" {
		t.Errorf("candidate = %+v", c)
	}
}

func TestHTMLListingDedupesLinks(t *testing.T) {
	f := &listingFixture{}
	srv := f.server(t)
	cfg := config.SourceConfig{Name: "A", URL: srv.URL + "/news", LinkPrefix: "/news/"}
	NewHTMLListing(cfg, Deps{HTTP: srv.Client()}).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if f.hits["/news/context-engineering"] != 1 {
		t.Errorf("article fetched %d times", f.hits["/news/context-engineering"])
	}
	if f.hits["/careers"] != 0 || f.hits["/news/"] != 0 {
		t.Errorf("fetched non-article links: %v", f.hits)
	}
}

func TestHTMLListingSkipsBrokenArticle(t *testing.T) {
	f := &listingFixture{}
	srv := f.server(t)
	cfg := config.SourceConfig{Name: "A", URL: srv.URL + "/news", LinkPrefix: "/news/"}
	_, err := NewHTMLListing(cfg, Deps{HTTP: srv.Client()}).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("broken article failed the source: %v", err)
	}
	if f.hits["/news/broken"] != 1 {
		t.Errorf("broken article not attempted")
	}
}

func TestBuildAllTypes(t *testing.T) {
	off := false
	cfgs := []config.SourceConfig{
		{Name: "a", Type: "rss", URL: "https://x"},
		{Name: "b", Type: "hackernews"},
		{Name: "c", Type: "reddit_rss", Subreddit: "ClaudeAI"},
		{Name: "d", Type: "html_listing", URL: "https://x", LinkPrefix: "/news/"},
		{Name: "e", Type: "rss", URL: "https://y", Enabled: &off},
	}
	srcs, err := Build(cfgs, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 4 {
		t.Fatalf("got %d sources", len(srcs))
	}
	for i, want := range []string{"a", "b", "c", "d"} {
		if srcs[i].Name() != want {
			t.Errorf("srcs[%d] = %s", i, srcs[i].Name())
		}
	}
}
