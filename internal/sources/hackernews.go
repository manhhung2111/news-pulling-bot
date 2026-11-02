package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/manhhung2111/news-pulling-bot/internal/config"
	"github.com/manhhung2111/news-pulling-bot/internal/model"
	"github.com/manhhung2111/news-pulling-bot/internal/urlnorm"
)

const hnItemURL = "https://news.ycombinator.com/item?id="

type hackerNews struct {
	cfg     config.SourceConfig
	d       Deps
	baseURL string
	kw      *regexp.Regexp
}

// NewHackerNews returns a source backed by the Algolia HN search API at baseURL.
func NewHackerNews(cfg config.SourceConfig, d Deps, baseURL string) Source {
	h := &hackerNews{cfg: cfg, d: d, baseURL: strings.TrimRight(baseURL, "/")}
	if len(cfg.Keywords) > 0 {
		parts := make([]string, len(cfg.Keywords))
		for i, k := range cfg.Keywords {
			parts[i] = regexp.QuoteMeta(k)
		}
		h.kw = regexp.MustCompile(`(?i)\b(` + strings.Join(parts, "|") + `)\b`)
	}
	return h
}

func (h *hackerNews) Name() string { return h.cfg.Name }

type hnHit struct {
	ObjectID  string `json:"objectID"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Points    int    `json:"points"`
	CreatedAt int64  `json:"created_at_i"`
	StoryText string `json:"story_text"`
}

func (h *hackerNews) Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error) {
	q := url.Values{}
	q.Set("tags", "story")
	q.Set("hitsPerPage", "200")
	q.Set("numericFilters", fmt.Sprintf("created_at_i>%d,points>=%d", since.Unix(), h.cfg.MinPoints))
	body, err := get(ctx, h.d, h.baseURL+"/api/v1/search_by_date?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Hits []hnHit `json:"hits"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode HN response: %w", err)
	}
	var out []model.Candidate
	for _, hit := range resp.Hits {
		if h.kw != nil && !h.kw.MatchString(hit.Title) {
			continue
		}
		item := hnItemURL + hit.ObjectID
		c := model.Candidate{
			Title:     hit.Title,
			URL:       hit.URL,
			Source:    h.cfg.Name,
			Published: time.Unix(hit.CreatedAt, 0),
			Excerpt:   plainText(hit.StoryText, 500),
			Points:    hit.Points,
		}
		if hit.URL == "" {
			c.URL = item
		} else {
			c.Discussion = []string{item}
		}
		c.NormURL = urlnorm.Normalize(c.URL)
		out = append(out, c)
	}
	return out, nil
}
