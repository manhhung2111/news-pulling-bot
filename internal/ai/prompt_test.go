package ai

import (
	"strings"
	"testing"

	"github.com/manhhung2111/news-pulling-bot/internal/model"
)

var testCands = []model.Candidate{
	{ID: "c1", Title: "Context hub for agents", URL: "https://a.com/hub", NormURL: "https://a.com/hub", Source: "HN",
		Discussion: []string{"https://news.ycombinator.com/item?id=1"}, Excerpt: "hub excerpt", ArticleText: "FULL ARTICLE BODY", CommunityTLDR: "people liked it"},
	{ID: "c2", Title: "PR review skill", URL: "https://b.com/pr", NormURL: "https://b.com/pr", Source: "PE", Paywalled: true},
}

func TestBuildSelectPromptContainsBarAndIDs(t *testing.T) {
	p := BuildSelectPrompt(testCands, "Owner likes Go.", 8)
	for _, want := range []string{QualityBar, "c1", "c2", "Context hub for agents", "hub excerpt", "Owner likes Go.", `"ids"`, "8"} {
		if !strings.Contains(p, want) {
			t.Errorf("select prompt missing %q", want)
		}
	}
	if strings.Contains(p, "FULL ARTICLE BODY") {
		t.Error("select prompt should not include article text")
	}
}

func TestBuildWritePromptIncludesArticleTextAndTLDR(t *testing.T) {
	p := BuildWritePrompt(testCands, "", 5)
	for _, want := range []string{QualityBar, "FULL ARTICLE BODY", "people liked it", "https://news.ycombinator.com/item?id=1",
		"try_this", "citations", "paywalled", `"items"`} {
		if !strings.Contains(p, want) {
			t.Errorf("write prompt missing %q", want)
		}
	}
}

func TestQualityBarMentionsKeyRules(t *testing.T) {
	for _, want := range []string{"concrete technique", "SDLC", "hype", "no AI angle", "zero items", "pad"} {
		if !strings.Contains(QualityBar, want) {
			t.Errorf("QualityBar missing %q", want)
		}
	}
}
