// Package discord renders the digest and talks to Discord.
package discord

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/mhung2111/ai-eng-digest/internal/model"
)

const (
	maxContent     = 2000
	maxTitle       = 256
	maxDescription = 4096
	maxFieldValue  = 1024
	maxEmbedTotal  = 6000
	embedColor     = 0xD97757
)

type Rendered struct {
	Headline   string
	ThreadName string
	Items      []*discordgo.MessageEmbed // one per item
}

// truncate caps s at n runes, ending with … when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:max(n, 0)])
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func Render(d model.Digest) Rendered {
	r := Rendered{ThreadName: "Digest — " + d.Date.Format("2006-01-02"), Headline: renderHeadline(d)}
	for _, it := range d.Items {
		r.Items = append(r.Items, renderEmbed(it))
	}
	return r
}

func renderHeadline(d model.Digest) string {
	date := d.Date.Format("Mon, Jan 2")
	var notes []string
	for _, s := range d.Unavailable {
		notes = append(notes, fmt.Sprintf("⚠️ %s unavailable today", s))
	}
	if d.Provider == "gemini" {
		notes = append(notes, "_Made with fallback model (Gemini)._")
	}
	noteBlock := ""
	if len(notes) > 0 {
		noteBlock = "\n" + strings.Join(notes, "\n")
	}
	if len(d.Items) == 0 {
		return truncate(fmt.Sprintf("☀️ %s — Nothing met the bar today.", date)+noteBlock, maxContent)
	}
	count := fmt.Sprintf("%d picks", len(d.Items))
	if len(d.Items) == 1 {
		count = "1 pick"
	}
	// Shrink hooks first, then titles, until the message fits.
	for _, lim := range [][2]int{{maxTitle, 1000}, {maxTitle, 200}, {maxTitle, 100}, {120, 100}, {120, 60}, {80, 0}, {50, 0}} {
		var b strings.Builder
		fmt.Fprintf(&b, "☀️ %s — %s\n", date, count)
		for i, it := range d.Items {
			fmt.Fprintf(&b, "%d. [%s](<%s>) — %s\n", i+1, truncate(it.Title, lim[0]), it.URL, it.Source)
			if lim[1] > 0 && it.Hook != "" {
				fmt.Fprintf(&b, "   %s\n", truncate(it.Hook, lim[1]))
			}
		}
		if noteBlock != "" {
			b.WriteString(noteBlock)
		}
		if s := b.String(); len([]rune(s)) <= maxContent {
			return s
		}
	}
	return truncate(fmt.Sprintf("☀️ %s — %s", date, count)+noteBlock, maxContent)
}

func renderEmbed(it model.Item) *discordgo.MessageEmbed {
	desc := it.Summary
	if it.Paywalled {
		desc = "🔒 partly paywalled\n" + desc
	}
	var steps []string
	for _, s := range it.TryThis {
		steps = append(steps, "• "+s)
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "✅ Try this", Value: strings.Join(steps, "\n")},
		{Name: "💡 Why it works", Value: it.WhyItWorks},
		{Name: "🎯 Problem it solves", Value: it.ProblemSolved},
	}
	if it.CommunityTLDR != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "💬 Community TL;DR", Value: it.CommunityTLDR})
	}
	if len(it.Discussion) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "🔗 Discussion", Value: strings.Join(it.Discussion, "\n")})
	}
	var cites []string
	for _, c := range it.Citations {
		label := c.Label
		if label == "" {
			label = "source"
		}
		cites = append(cites, fmt.Sprintf("[%d] [%s](%s)", c.N, label, c.URL))
	}
	fields = append(fields, &discordgo.MessageEmbedField{Name: "📚 Sources", Value: strings.Join(cites, "\n")})

	e := &discordgo.MessageEmbed{
		Title:  truncate(it.Title, maxTitle),
		URL:    it.URL,
		Color:  embedColor,
		Footer: &discordgo.MessageEmbedFooter{Text: it.Source},
	}
	// Leave at least minDescription for the summary; share the rest of the 6000-char budget across fields.
	const minDescription = 500
	fixed := len([]rune(e.Title)) + len([]rune(it.Source))
	for _, f := range fields {
		fixed += len([]rune(f.Name))
	}
	fieldCap := maxFieldValue
	if share := (maxEmbedTotal - fixed - minDescription) / len(fields); share < fieldCap {
		fieldCap = share
	}
	total := fixed
	for _, f := range fields {
		if f.Value == "" {
			f.Value = "—"
		}
		f.Value = truncate(f.Value, fieldCap)
		total += len([]rune(f.Value))
	}
	descLimit := maxDescription
	if room := maxEmbedTotal - total; room < descLimit {
		descLimit = room
	}
	e.Description = truncate(desc, descLimit)
	e.Fields = fields
	return e
}

// RenderFailure is the one-line message posted when a run fails.
func RenderFailure(reason string) string {
	return fmt.Sprintf("❌ Digest failed today: %s. Details are in the server log.", truncate(strings.TrimSuffix(strings.TrimSpace(reason), "."), 300))
}
