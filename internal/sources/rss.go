package sources

import (
	"context"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
	"github.com/manhhung2111/news-pulling-bot/internal/model"
	"github.com/manhhung2111/news-pulling-bot/internal/urlnorm"
)

type rssSource struct {
	cfg config.SourceConfig
	d   Deps
}

// NewRSS returns a source for a generic RSS/Atom feed.
func NewRSS(cfg config.SourceConfig, d Deps) Source { return &rssSource{cfg: cfg, d: d} }

func (s *rssSource) Name() string { return s.cfg.Name }

func (s *rssSource) Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error) {
	feed, err := fetchFeed(ctx, s.d, s.cfg.URL)
	if err != nil {
		return nil, err
	}
	var out []model.Candidate
	for _, it := range feed.Items {
		pub := entryTime(it)
		if pub.IsZero() || pub.Before(since) || it.Link == "" {
			continue
		}
		excerpt := it.Description
		if excerpt == "" {
			excerpt = it.Content
		}
		out = append(out, model.Candidate{
			Title:     plainText(it.Title, 300),
			URL:       it.Link,
			NormURL:   urlnorm.Normalize(it.Link),
			Source:    s.cfg.Name,
			Published: pub,
			Excerpt:   plainText(excerpt, 500),
			Paywalled: s.cfg.Paywalled,
		})
	}
	return out, nil
}

func fetchFeed(ctx context.Context, d Deps, url string) (*gofeed.Feed, error) {
	body, err := get(ctx, d, url)
	if err != nil {
		return nil, err
	}
	return gofeed.NewParser().ParseString(string(body))
}

func entryTime(it *gofeed.Item) time.Time {
	if it.PublishedParsed != nil {
		return *it.PublishedParsed
	}
	if it.UpdatedParsed != nil {
		return *it.UpdatedParsed
	}
	return time.Time{}
}
