package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
	"github.com/manhhung2111/news-pulling-bot/internal/model"
)

func hnServer(t *testing.T, check func(r *http.Request)) *httptest.Server {
	t.Helper()
	ts := time.Now().Add(-2 * time.Hour).Unix()
	body := fmt.Sprintf(`{"hits":[
	 {"objectID":"1","title":"Using Claude Code for PR review","url":"https://blog.example.com/pr-review","points":120,"created_at_i":%d},
	 {"objectID":"2","title":"He said hello","url":"https://example.com/hello","points":200,"created_at_i":%d},
	 {"objectID":"3","title":"Ask HN: agents in CI?","url":"","points":45,"created_at_i":%d,"story_text":"<p>How do you run agents in CI?</p>"},
	 {"objectID":"4","title":"Rust 2.0 released","url":"https://rust.example.com","points":900,"created_at_i":%d}
	]}`, ts, ts, ts, ts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

var hnCfg = config.SourceConfig{Name: "Hacker News", Type: "hackernews", MinPoints: 30,
	Keywords: []string{"Claude", "agent", "agents", "AI"}}

func fetchHN(t *testing.T, srv *httptest.Server) []model.Candidate {
	t.Helper()
	got, err := NewHackerNews(hnCfg, Deps{HTTP: srv.Client()}, srv.URL).Fetch(context.Background(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestHackerNewsKeywordFilterWholeWord(t *testing.T) {
	got := fetchHN(t, hnServer(t, nil))
	if len(got) != 2 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Title != "Using Claude Code for PR review" || c.URL != "https://blog.example.com/pr-review" ||
		c.Points != 120 || len(c.Discussion) != 1 || c.Discussion[0] != "https://news.ycombinator.com/item?id=1" {
		t.Errorf("first = %+v", c)
	}
}

func TestHackerNewsAskHNUsesItemURL(t *testing.T) {
	got := fetchHN(t, hnServer(t, nil))
	c := got[1]
	if c.URL != "https://news.ycombinator.com/item?id=3" || len(c.Discussion) != 0 || c.Excerpt != "How do you run agents in CI?" {
		t.Errorf("ask HN = %+v", c)
	}
}

func TestHackerNewsQueryParams(t *testing.T) {
	fetchHN(t, hnServer(t, func(r *http.Request) {
		if r.URL.Path != "/api/v1/search_by_date" {
			t.Errorf("path = %s", r.URL.Path)
		}
		nf := r.URL.Query().Get("numericFilters")
		if !strings.Contains(nf, "points>=30") || !strings.Contains(nf, "created_at_i>") {
			t.Errorf("numericFilters = %q", nf)
		}
		if r.URL.Query().Get("tags") != "story" {
			t.Errorf("tags = %q", r.URL.Query().Get("tags"))
		}
	}))
}
