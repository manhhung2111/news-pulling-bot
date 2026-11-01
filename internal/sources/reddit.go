package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/config"
	"github.com/mhung2111/ai-eng-digest/internal/model"
	"github.com/mhung2111/ai-eng-digest/internal/urlnorm"
)

// Reddit reads a subreddit through its public RSS feeds (no API key).
type Reddit struct {
	cfg     config.SourceConfig
	d       Deps
	baseURL string
}

var redditLinkRe = regexp.MustCompile(`<a href="([^"]+)">\[link\]</a>`)

// NewReddit returns a Reddit source. baseURL is https://www.reddit.com in production.
func NewReddit(cfg config.SourceConfig, d Deps, baseURL string) *Reddit {
	if cfg.Listing == "" {
		cfg.Listing = "top"
	}
	if cfg.Period == "" {
		cfg.Period = "day"
	}
	return &Reddit{cfg: cfg, d: d, baseURL: strings.TrimRight(baseURL, "/")}
}

func (r *Reddit) Name() string { return r.cfg.Name }

func (r *Reddit) Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error) {
	u := fmt.Sprintf("%s/r/%s/%s/.rss?t=%s", r.baseURL, r.cfg.Subreddit, r.cfg.Listing, url.QueryEscape(r.cfg.Period))
	feed, err := fetchFeed(ctx, r.d, u)
	if err != nil {
		return nil, err
	}
	var out []model.Candidate
	for _, it := range feed.Items {
		pub := entryTime(it)
		if pub.IsZero() || pub.Before(since) || it.Link == "" {
			continue
		}
		permalink := it.Link
		c := model.Candidate{
			Title:           plainText(it.Title, 300),
			URL:             permalink,
			Source:          r.cfg.Name,
			Published:       pub,
			RedditPermalink: permalink,
		}
		if m := redditLinkRe.FindStringSubmatch(it.Content); m != nil && !isRedditURL(html.UnescapeString(m[1])) {
			c.URL = html.UnescapeString(m[1])
			c.Discussion = []string{permalink}
		}
		body, _, _ := strings.Cut(it.Content, "submitted by")
		c.Excerpt = plainText(body, 500)
		c.NormURL = urlnorm.Normalize(c.URL)
		out = append(out, c)
	}
	return out, nil
}

func isRedditURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return true
	}
	h := strings.ToLower(u.Host)
	return h == "redd.it" || strings.HasSuffix(h, "reddit.com") || strings.HasSuffix(h, "redd.it")
}

// FetchTLDR returns the summary comment left by the subreddit's bot on the thread at permalink, or "" when there is none.
func (r *Reddit) FetchTLDR(ctx context.Context, permalink string) (string, error) {
	p, err := url.Parse(permalink)
	if err != nil {
		return "", err
	}
	u := r.baseURL + strings.TrimRight(p.Path, "/") + "/.rss"
	feed, err := fetchFeed(ctx, r.d, u)
	if err != nil {
		return "", err
	}
	author := strings.ToLower(strings.TrimPrefix(r.cfg.TLDRAuthor, "/u/"))
	for _, it := range feed.Items {
		if strings.HasPrefix(it.GUID, "t3_") { // the post itself, not a comment
			continue
		}
		name := ""
		if it.Author != nil {
			name = strings.ToLower(strings.TrimPrefix(it.Author.Name, "/u/"))
		}
		text := plainText(it.Content, 600)
		byAuthor := author != "" && name == author
		byMarker := r.cfg.TLDRMarker != "" && strings.Contains(text, r.cfg.TLDRMarker)
		if byAuthor || byMarker {
			return text, nil
		}
	}
	return "", nil
}
