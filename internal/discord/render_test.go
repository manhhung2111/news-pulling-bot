package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/mhung2111/ai-eng-digest/internal/model"
)

var day = time.Date(2026, 11, 1, 8, 0, 0, 0, time.FixedZone("ICT", 7*3600))

func item(n int) model.Item {
	return model.Item{
		Title: "Title " + string(rune('A'+n)), URL: "https://ex.com/" + string(rune('a'+n)), Source: "HN", Hook: "Why open it",
		Summary: "Summary [1].", TryThis: []string{"Step one", "Step two"}, WhyItWorks: "Because [1].", ProblemSolved: "Forgetting [1].",
		Citations: []model.Citation{{N: 1, URL: "https://ex.com/a", Label: "article"}},
	}
}

func TestRenderHeadlineThreePicks(t *testing.T) {
	r := Render(model.Digest{Date: day, Items: []model.Item{item(0), item(1), item(2)}, Provider: "claude"})
	want := "☀️ Sun, Nov 1 — 3 picks\n1. [Title A](<https://ex.com/a>) — HN\n   Why open it\n"
	if !strings.HasPrefix(r.Headline, want) {
		t.Fatalf("headline =\n%s", r.Headline)
	}
	if r.ThreadName != "Digest — 2026-11-01" || len(r.Items) != 3 {
		t.Errorf("thread=%q items=%d", r.ThreadName, len(r.Items))
	}
	if strings.Contains(r.Headline, "fallback") {
		t.Error("claude digest should not carry fallback note")
	}
}

func TestRenderHeadlineOnePick(t *testing.T) {
	r := Render(model.Digest{Date: day, Items: []model.Item{item(0)}})
	if !strings.HasPrefix(r.Headline, "☀️ Sun, Nov 1 — 1 pick\n") {
		t.Fatalf("headline = %q", r.Headline)
	}
}

func TestRenderZeroItems(t *testing.T) {
	r := Render(model.Digest{Date: day})
	if r.Headline != "☀️ Sun, Nov 1 — Nothing met the bar today." || len(r.Items) != 0 {
		t.Fatalf("headline = %q items=%d", r.Headline, len(r.Items))
	}
}

func TestRenderUnavailableAndFallbackNotes(t *testing.T) {
	r := Render(model.Digest{Date: day, Items: []model.Item{item(0)}, Unavailable: []string{"r/ClaudeAI"}, Provider: "gemini"})
	if !strings.Contains(r.Headline, "\n\n⚠️ r/ClaudeAI unavailable today\n_Made with fallback model (Gemini)._") {
		t.Fatalf("headline = %q", r.Headline)
	}
}

func fieldNames(e *discordgo.MessageEmbed) []string {
	var out []string
	for _, f := range e.Fields {
		out = append(out, f.Name)
	}
	return out
}

func TestRenderEmbedFieldsAndOrder(t *testing.T) {
	it := item(0)
	it.CommunityTLDR = "Mostly positive."
	it.Discussion = []string{"https://news.ycombinator.com/item?id=1"}
	e := Render(model.Digest{Date: day, Items: []model.Item{it, item(1)}}).Items[0]
	if e.Title != "Title A" || e.URL != "https://ex.com/a" || e.Description != "Summary [1]." || e.Color != 0xD97757 || e.Footer.Text != "HN" {
		t.Errorf("embed = %+v", e)
	}
	got := strings.Join(fieldNames(e), "|")
	if got != "✅ Try this|💡 Why it works|🎯 Problem it solves|💬 Community TL;DR|🔗 Discussion|📚 Sources" {
		t.Errorf("fields = %s", got)
	}
	if e.Fields[0].Value != "• Step one\n• Step two" || e.Fields[5].Value != "[1] [article](https://ex.com/a)" {
		t.Errorf("values = %q / %q", e.Fields[0].Value, e.Fields[5].Value)
	}
	plain := Render(model.Digest{Date: day, Items: []model.Item{item(1)}}).Items[0]
	if strings.Join(fieldNames(plain), "|") != "✅ Try this|💡 Why it works|🎯 Problem it solves|📚 Sources" {
		t.Errorf("optional fields present: %v", fieldNames(plain))
	}
}

func TestRenderPaywallLabel(t *testing.T) {
	it := item(0)
	it.Paywalled = true
	e := Render(model.Digest{Date: day, Items: []model.Item{it}}).Items[0]
	if !strings.HasPrefix(e.Description, "🔒 partly paywalled\n") {
		t.Fatalf("description = %q", e.Description)
	}
}

func embedChars(e *discordgo.MessageEmbed) int {
	n := len([]rune(e.Title)) + len([]rune(e.Description)) + len([]rune(e.Footer.Text))
	for _, f := range e.Fields {
		n += len([]rune(f.Name)) + len([]rune(f.Value))
	}
	return n
}

func TestRenderTruncatesLongFields(t *testing.T) {
	it := item(0)
	long := strings.Repeat("x", 5000)
	it.TryThis = []string{long}
	it.WhyItWorks, it.ProblemSolved, it.CommunityTLDR = long, long, long
	it.Summary = strings.Repeat("y", 5000)
	e := Render(model.Digest{Date: day, Items: []model.Item{it}}).Items[0]
	for _, f := range e.Fields {
		if len([]rune(f.Value)) > 1024 {
			t.Errorf("field %s has %d chars", f.Name, len([]rune(f.Value)))
		}
	}
	if len([]rune(e.Description)) > 4096 || embedChars(e) > 6000 {
		t.Errorf("description=%d total=%d", len([]rune(e.Description)), embedChars(e))
	}
	if !strings.HasSuffix(e.Fields[1].Value, "…") {
		t.Error("truncated field should end with …")
	}
}

func TestRenderHeadlineUnder2000(t *testing.T) {
	var items []model.Item
	for i := 0; i < 5; i++ {
		it := item(i)
		it.Hook = strings.Repeat("h", 600)
		it.Title = strings.Repeat("t", 250)
		items = append(items, it)
	}
	r := Render(model.Digest{Date: day, Items: items, Unavailable: []string{"r/ClaudeAI"}, Provider: "gemini"})
	if n := len([]rune(r.Headline)); n > 2000 {
		t.Fatalf("headline has %d chars", n)
	}
	if !strings.Contains(r.Headline, "⚠️ r/ClaudeAI unavailable today") || !strings.Contains(r.Headline, "5. [") {
		t.Error("headline lost notes or items while shrinking")
	}
}

func TestRenderFailure(t *testing.T) {
	got := RenderFailure("all AI providers failed")
	if got != "❌ Digest failed today: all AI providers failed. Details are in the server log." {
		t.Fatalf("got %q", got)
	}
	if n := len([]rune(RenderFailure(strings.Repeat("e", 1000)))); n > 400 {
		t.Fatalf("failure message too long: %d", n)
	}
}

func TestRenderWorstCaseAllFieldsMaxed(t *testing.T) {
	it := item(0)
	long := strings.Repeat("x", 5000)
	it.Title, it.Summary, it.WhyItWorks, it.ProblemSolved, it.CommunityTLDR = long, long, long, long, long
	it.TryThis = []string{long}
	it.Discussion = []string{"https://news.ycombinator.com/item?id=1" + long}
	for i := 0; i < 40; i++ {
		it.Citations = append(it.Citations, model.Citation{N: i, URL: "https://ex.com/a", Label: "article"})
	}
	e := Render(model.Digest{Date: day, Items: []model.Item{it}}).Items[0]
	if embedChars(e) > 6000 || len([]rune(e.Description)) < 100 {
		t.Fatalf("total=%d description=%d", embedChars(e), len([]rune(e.Description)))
	}
}
