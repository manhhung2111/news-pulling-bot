// Package enrich fetches extra context for shortlisted candidates.
package enrich

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-shiori/go-readability"
)

const maxArticleChars = 8000

var spaceRe = regexp.MustCompile(`\s+`)

// ArticleText fetches pageURL and returns its main readable text, capped at 8000 characters.
func ArticleText(ctx context.Context, hc *http.Client, ua, pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", pageURL, resp.StatusCode)
	}
	art, err := readability.FromReader(io.LimitReader(resp.Body, 5<<20), u)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(spaceRe.ReplaceAllString(art.TextContent, " "))
	if r := []rune(text); len(r) > maxArticleChars {
		text = string(r[:maxArticleChars-1]) + "…"
	}
	return text, nil
}
