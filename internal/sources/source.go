// Package sources collects candidate posts from configured feeds and sites.
package sources

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/config"
	"github.com/mhung2111/ai-eng-digest/internal/model"
)

type Source interface {
	Name() string
	Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error)
}

type Deps struct {
	HTTP      *http.Client
	UserAgent string
}

const fetchTimeout = 30 * time.Second

// Build constructs the enabled sources from config.
func Build(cfgs []config.SourceConfig, d Deps) ([]Source, error) {
	var out []Source
	for _, c := range cfgs {
		if !c.IsEnabled() {
			continue
		}
		switch c.Type {
		case "rss":
			out = append(out, NewRSS(c, d))
		default:
			return nil, fmt.Errorf("source %q: unsupported type %q", c.Name, c.Type)
		}
	}
	return out, nil
}

// CollectAll fetches every source concurrently. A failing source is reported in its status and never stops the others.
// Candidates and statuses keep source order.
func CollectAll(ctx context.Context, srcs []Source, since time.Time) ([]model.Candidate, []model.SourceStatus) {
	results := make([][]model.Candidate, len(srcs))
	statuses := make([]model.SourceStatus, len(srcs))
	var wg sync.WaitGroup
	for i, s := range srcs {
		wg.Add(1)
		go func(i int, s Source) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
			cands, err := s.Fetch(cctx, since)
			st := model.SourceStatus{Name: s.Name(), OK: err == nil, Count: len(cands)}
			if err != nil {
				st.Err = err.Error()
				st.Count = 0
				cands = nil
			}
			results[i], statuses[i] = cands, st
		}(i, s)
	}
	wg.Wait()
	var all []model.Candidate
	for _, r := range results {
		all = append(all, r...)
	}
	return all, statuses
}

// get performs a GET with the configured User-Agent and returns the body of a 200 response.
func get(ctx context.Context, d Deps, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if d.UserAgent != "" {
		req.Header.Set("User-Agent", d.UserAgent)
	}
	hc := d.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

var (
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// plainText strips HTML tags, unescapes entities, collapses whitespace and caps the result at max runes.
func plainText(s string, max int) string {
	s = tagRe.ReplaceAllString(s, " ")
	for i := 0; i < 2 && strings.Contains(s, "&"); i++ { // feeds often double-escape
		s = html.UnescapeString(s)
	}
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max-1])) + "…"
	}
	return s
}
