// Package pipeline runs one digest: collect, filter, pick, write, post, record.
package pipeline

import (
	"fmt"
	"sort"

	"github.com/mhung2111/ai-eng-digest/internal/model"
	"github.com/mhung2111/ai-eng-digest/internal/urlnorm"
)

type SeenChecker interface {
	IsSent(string) (bool, error)
	IsSeen(string) (bool, error)
}

// isAggregator reports whether c came from a discussion site (HN, Reddit) rather than the original publisher.
func isAggregator(c model.Candidate) bool {
	return c.Points > 0 || c.RedditPermalink != "" || len(c.Discussion) > 0
}

// MergeCandidates collapses candidates pointing at the same article into one, preferring the original
// publisher as primary and keeping every discussion link. Output is newest first.
func MergeCandidates(cs []model.Candidate) []model.Candidate {
	var order []string
	groups := map[string][]model.Candidate{}
	for _, c := range cs {
		if c.NormURL == "" {
			continue
		}
		if _, ok := groups[c.NormURL]; !ok {
			order = append(order, c.NormURL)
		}
		groups[c.NormURL] = append(groups[c.NormURL], c)
	}
	out := make([]model.Candidate, 0, len(order))
	for _, key := range order {
		g := groups[key]
		primary := g[0]
		for _, c := range g {
			if !isAggregator(c) {
				primary = c
				break
			}
		}
		merged := primary
		merged.Discussion = nil
		seenDisc := map[string]bool{}
		for _, c := range g {
			for _, d := range c.Discussion {
				if n := urlnorm.Normalize(d); n != "" && !seenDisc[n] {
					seenDisc[n] = true
					merged.Discussion = append(merged.Discussion, d)
				}
			}
			if c.Points > merged.Points {
				merged.Points = c.Points
			}
			if merged.RedditPermalink == "" {
				merged.RedditPermalink = c.RedditPermalink
			}
			if c.Published.Before(merged.Published) {
				merged.Published = c.Published
			}
			if merged.Excerpt == "" {
				merged.Excerpt = c.Excerpt
			}
		}
		out = append(out, merged)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	return out
}

// FilterNew drops candidates already sent (by article or discussion URL) or already shown to the AI.
func FilterNew(cs []model.Candidate, st SeenChecker) ([]model.Candidate, error) {
	var out []model.Candidate
	for _, c := range cs {
		urls := []string{c.NormURL}
		for _, d := range c.Discussion {
			urls = append(urls, urlnorm.Normalize(d))
		}
		drop := false
		for _, u := range urls {
			sent, err := st.IsSent(u)
			if err != nil {
				return nil, err
			}
			if sent {
				drop = true
				break
			}
		}
		if !drop {
			seen, err := st.IsSeen(c.NormURL)
			if err != nil {
				return nil, err
			}
			drop = seen
		}
		if !drop {
			out = append(out, c)
		}
	}
	return out, nil
}

// AssignIDs gives each candidate a run-local ID "c1".."cN".
func AssignIDs(cs []model.Candidate) {
	for i := range cs {
		cs[i].ID = fmt.Sprintf("c%d", i+1)
	}
}
