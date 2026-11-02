package pipeline

import (
	"testing"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/model"
	"github.com/manhhung2111/news-pulling-bot/internal/urlnorm"
)

func cand(src, url string, opts ...func(*model.Candidate)) model.Candidate {
	c := model.Candidate{Title: src + " " + url, URL: url, NormURL: urlnorm.Normalize(url), Source: src, Published: time.Now()}
	for _, o := range opts {
		o(&c)
	}
	return c
}

func TestMergeCollapsesSameArticle(t *testing.T) {
	now := time.Now()
	in := []model.Candidate{
		cand("Hacker News", "https://anthropic.com/news/x?utm_source=hn", func(c *model.Candidate) {
			c.Points = 300
			c.Discussion = []string{"https://news.ycombinator.com/item?id=9"}
			c.Published = now.Add(-time.Hour)
		}),
		cand("Anthropic News", "https://www.anthropic.com/news/x", func(c *model.Candidate) { c.Published = now.Add(-3 * time.Hour) }),
		cand("r/ClaudeAI", "https://anthropic.com/news/x/", func(c *model.Candidate) {
			c.RedditPermalink = "https://www.reddit.com/r/ClaudeAI/comments/a/x/"
			c.Discussion = []string{c.RedditPermalink}
		}),
	}
	got := MergeCandidates(in)
	if len(got) != 1 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Source != "Anthropic News" || c.URL != "https://www.anthropic.com/news/x" || c.Points != 300 ||
		len(c.Discussion) != 2 || c.RedditPermalink == "" || !c.Published.Equal(now.Add(-3*time.Hour)) {
		t.Errorf("merged = %+v", c)
	}
}

func TestMergeKeepsDistinctNewestFirst(t *testing.T) {
	now := time.Now()
	got := MergeCandidates([]model.Candidate{
		cand("A", "https://a.com/1", func(c *model.Candidate) { c.Published = now.Add(-2 * time.Hour) }),
		cand("B", "https://b.com/2", func(c *model.Candidate) { c.Published = now }),
		{Title: "bad", URL: "mailto:x", NormURL: ""},
	})
	if len(got) != 2 || got[0].Source != "B" {
		t.Fatalf("got %+v", got)
	}
}

type fakeChecker struct{ sent, seen map[string]bool }

func (f fakeChecker) IsSent(u string) (bool, error) { return f.sent[u], nil }
func (f fakeChecker) IsSeen(u string) (bool, error) { return f.seen[u], nil }

func TestFilterDropsSentAndSeen(t *testing.T) {
	in := []model.Candidate{cand("A", "https://a.com/1"), cand("A", "https://a.com/2"), cand("A", "https://a.com/3")}
	chk := fakeChecker{sent: map[string]bool{"https://a.com/1": true}, seen: map[string]bool{"https://a.com/2": true}}
	got, err := FilterNew(in, chk)
	if err != nil || len(got) != 1 || got[0].URL != "https://a.com/3" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFilterDropsWhenDiscussionAlreadySent(t *testing.T) {
	in := []model.Candidate{cand("HN", "https://a.com/new", func(c *model.Candidate) {
		c.Discussion = []string{"https://news.ycombinator.com/item?id=1"}
	})}
	chk := fakeChecker{sent: map[string]bool{"https://news.ycombinator.com/item?id=1": true}}
	got, _ := FilterNew(in, chk)
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAssignIDs(t *testing.T) {
	cs := []model.Candidate{{}, {}, {}}
	AssignIDs(cs)
	if cs[0].ID != "c1" || cs[2].ID != "c3" {
		t.Fatalf("ids = %s %s", cs[0].ID, cs[2].ID)
	}
}
