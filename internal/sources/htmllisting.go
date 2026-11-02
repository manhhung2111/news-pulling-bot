package sources

import (
	"bytes"
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
	"github.com/manhhung2111/news-pulling-bot/internal/model"
	"github.com/manhhung2111/news-pulling-bot/internal/urlnorm"
)

const maxListingLinks = 15

// htmlListing watches a blog index page (e.g. anthropic.com/news) that has no feed.
type htmlListing struct {
	cfg config.SourceConfig
	d   Deps
}

func NewHTMLListing(cfg config.SourceConfig, d Deps) Source { return &htmlListing{cfg: cfg, d: d} }

func (h *htmlListing) Name() string { return h.cfg.Name }

func (h *htmlListing) Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error) {
	base, err := url.Parse(h.cfg.URL)
	if err != nil {
		return nil, err
	}
	body, err := get(ctx, h.d, h.cfg.URL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var links []string
	seen := map[string]bool{}
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		ref, err := url.Parse(href)
		if err != nil {
			return true
		}
		abs := base.ResolveReference(ref)
		abs.Fragment = ""
		if abs.Host != base.Host || !strings.HasPrefix(abs.Path, h.cfg.LinkPrefix) ||
			len(strings.TrimRight(abs.Path, "/")) <= len(strings.TrimRight(h.cfg.LinkPrefix, "/")) {
			return true
		}
		if s := abs.String(); !seen[s] {
			seen[s] = true
			links = append(links, s)
		}
		return len(links) < maxListingLinks
	})

	var out []model.Candidate
	for _, link := range links {
		c, ok := h.article(ctx, link)
		if !ok || c.Published.Before(since) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

var ldDateRe = regexp.MustCompile(`"datePublished"\s*:\s*"([^"]+)"`)

// article fetches one page and reads its title and published date. ok is false when the page fails or has no date.
func (h *htmlListing) article(ctx context.Context, link string) (model.Candidate, bool) {
	body, err := get(ctx, h.d, link)
	if err != nil {
		return model.Candidate{}, false
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return model.Candidate{}, false
	}
	var pub time.Time
	if v, ok := doc.Find(`meta[property="article:published_time"]`).Attr("content"); ok {
		pub = parseDate(v)
	}
	if pub.IsZero() {
		doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
			if m := ldDateRe.FindStringSubmatch(s.Text()); m != nil {
				pub = parseDate(m[1])
			}
			return pub.IsZero()
		})
	}
	if pub.IsZero() {
		if v, ok := doc.Find("time[datetime]").First().Attr("datetime"); ok {
			pub = parseDate(v)
		}
	}
	if pub.IsZero() {
		return model.Candidate{}, false
	}
	title, _ := doc.Find(`meta[property="og:title"]`).Attr("content")
	if title == "" {
		title = doc.Find("title").First().Text()
	}
	desc, _ := doc.Find(`meta[name="description"]`).Attr("content")
	return model.Candidate{
		Title:     plainText(title, 300),
		URL:       link,
		NormURL:   urlnorm.Normalize(link),
		Source:    h.cfg.Name,
		Published: pub,
		Excerpt:   plainText(desc, 500),
	}, true
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
