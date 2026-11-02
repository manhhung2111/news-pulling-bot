package ai

import (
	"testing"

	"github.com/manhhung2111/news-pulling-bot/internal/model"
)

func TestParseShortlistStripsFenceAndUnknownIDs(t *testing.T) {
	raw := "```json\n{\"ids\":[\"c2\",\"c9\",\"c1\",\"c2\"]}\n```"
	got, err := ParseShortlist(raw, map[string]bool{"c1": true, "c2": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "c2" || got[1] != "c1" {
		t.Fatalf("got %v", got)
	}
}

func TestParseItemsInvalidJSONErrors(t *testing.T) {
	if _, err := ParseItems("not json"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseItemsWithProse(t *testing.T) {
	got, err := ParseItems("Here you go:\n```\n{\"items\":[{\"candidate_id\":\"c1\",\"summary\":\"s\"}]}\n```")
	if err != nil || len(got) != 1 || got[0].CandidateID != "c1" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func byID() map[string]model.Candidate {
	m := map[string]model.Candidate{}
	for _, c := range testCands {
		m[c.ID] = c
	}
	return m
}

func goodItem(id, url string) model.Item {
	return model.Item{CandidateID: id, Title: "T", URL: "https://evil.com", Source: "X", Summary: "Does a thing [1].",
		TryThis: []string{"step"}, Citations: []model.Citation{{N: 1, URL: url, Label: "article"}}}
}

func TestValidateDropsUnknownCandidate(t *testing.T) {
	got := ValidateItems([]model.Item{goodItem("c9", "https://a.com/hub")}, byID(), 5)
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateOverwritesURLFromCandidate(t *testing.T) {
	got := ValidateItems([]model.Item{goodItem("c1", "https://a.com/hub?utm_source=x")}, byID(), 5)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	it := got[0]
	if it.URL != "https://a.com/hub" || it.Source != "HN" || len(it.Discussion) != 1 || it.CommunityTLDR != "people liked it" {
		t.Errorf("item = %+v", it)
	}
}

func TestValidateCopiesPaywalled(t *testing.T) {
	got := ValidateItems([]model.Item{goodItem("c2", "https://b.com/pr")}, byID(), 5)
	if len(got) != 1 || !got[0].Paywalled {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateDropsForeignCitations(t *testing.T) {
	it := goodItem("c1", "https://a.com/hub")
	it.Citations = append(it.Citations, model.Citation{N: 2, URL: "https://made-up.com/x"},
		model.Citation{N: 3, URL: "https://news.ycombinator.com/item?id=1"})
	got := ValidateItems([]model.Item{it}, byID(), 5)
	if len(got) != 1 || len(got[0].Citations) != 2 || got[0].Citations[1].N != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateDropsItemWithNoCitations(t *testing.T) {
	got := ValidateItems([]model.Item{goodItem("c1", "https://made-up.com")}, byID(), 5)
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateDropsEmptySummaryOrSteps(t *testing.T) {
	a := goodItem("c1", "https://a.com/hub")
	a.Summary = ""
	b := goodItem("c1", "https://a.com/hub")
	b.TryThis = nil
	if got := ValidateItems([]model.Item{a, b}, byID(), 5); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateTruncatesToMax(t *testing.T) {
	items := []model.Item{goodItem("c1", "https://a.com/hub"), goodItem("c2", "https://b.com/pr")}
	if got := ValidateItems(items, byID(), 1); len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
}

func TestValidateDropsDuplicateCandidate(t *testing.T) {
	items := []model.Item{goodItem("c1", "https://a.com/hub"), goodItem("c1", "https://a.com/hub")}
	if got := ValidateItems(items, byID(), 5); len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
}
