package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manhhung2111/news-pulling-bot/internal/model"
	"github.com/manhhung2111/news-pulling-bot/internal/urlnorm"
)

// extractJSON strips a ``` fence or surrounding prose and returns the outermost JSON object.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

// ParseShortlist parses {"ids":[...]}, dropping unknown and duplicate IDs and keeping order.
func ParseShortlist(raw string, valid map[string]bool) ([]string, error) {
	var resp struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return nil, fmt.Errorf("parse shortlist: %w", err)
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range resp.IDs {
		if valid[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// ParseItems parses {"items":[...]}.
func ParseItems(raw string) ([]model.Item, error) {
	var resp struct {
		Items []model.Item `json:"items"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return nil, fmt.Errorf("parse items: %w", err)
	}
	return resp.Items, nil
}

// ValidateItems keeps only items that refer to a known candidate and cite only that candidate's URLs.
// URL, Source, Discussion and Paywalled always come from the candidate, never from the AI.
func ValidateItems(items []model.Item, byID map[string]model.Candidate, maxItems int) []model.Item {
	var out []model.Item
	used := map[string]bool{}
	for _, it := range items {
		c, ok := byID[it.CandidateID]
		if !ok || used[it.CandidateID] {
			continue
		}
		it.URL, it.Source, it.Discussion, it.Paywalled = c.URL, c.Source, c.Discussion, c.Paywalled
		if it.Title == "" {
			it.Title = c.Title
		}
		if it.CommunityTLDR == "" {
			it.CommunityTLDR = c.CommunityTLDR
		}
		allowed := map[string]bool{c.NormURL: true}
		for _, d := range c.Discussion {
			allowed[urlnorm.Normalize(d)] = true
		}
		var cites []model.Citation
		for _, ct := range it.Citations {
			if allowed[urlnorm.Normalize(ct.URL)] {
				cites = append(cites, ct)
			}
		}
		it.Citations = cites
		if strings.TrimSpace(it.Summary) == "" || len(it.TryThis) == 0 || len(cites) == 0 {
			continue
		}
		used[it.CandidateID] = true
		out = append(out, it)
		if len(out) == maxItems {
			break
		}
	}
	return out
}
