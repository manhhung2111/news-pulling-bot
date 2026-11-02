// Package ai builds prompts, calls the AI providers and validates what comes back.
package ai

import (
	"fmt"
	"strings"

	"github.com/manhhung2111/news-pulling-bot/internal/model"
)

// QualityBar is the fixed bar every pick must clear. Owner feedback steers choices within it, never replaces it.
const QualityBar = `You are curating a daily digest for a working software engineer about applying AI coding agents across the software development lifecycle (SDLC).

Pick an item only if it teaches a concrete technique the reader could apply, either:
- to their coding-agent setup: tools, context, knowledge, skills or packaged workflows, memory, decision logs; or
- to using agents at a stage of the SDLC: specs and requirements, design, implementation, code review, testing, CI/CD and release, incidents and debugging, documentation, migrations and maintenance, team process.

Reject: hype, opinion-only pieces, model or product launch announcements without practices, benchmark races, funding news, beginner content, and general software engineering with no AI angle. Popularity is not quality: a high-score thread of hot takes loses to a modest post with a concrete technique.

Fewer is better than weak. Returning zero items is correct when nothing qualifies. Never pad the list to reach a number.`

func writeCandidates(b *strings.Builder, cs []model.Candidate, full bool) {
	for _, c := range cs {
		fmt.Fprintf(b, "\n### %s\nTitle: %s\nURL: %s\nSource: %s\n", c.ID, c.Title, c.URL, c.Source)
		if len(c.Discussion) > 0 {
			fmt.Fprintf(b, "Discussion: %s\n", strings.Join(c.Discussion, " , "))
		}
		if c.Points > 0 {
			fmt.Fprintf(b, "HN points: %d\n", c.Points)
		}
		if c.Paywalled {
			b.WriteString("Note: partly paywalled; only the visible text is available.\n")
		}
		if c.Excerpt != "" {
			fmt.Fprintf(b, "Excerpt: %s\n", c.Excerpt)
		}
		if full {
			if c.CommunityTLDR != "" {
				fmt.Fprintf(b, "Community TL;DR (Reddit): %s\n", c.CommunityTLDR)
			}
			if c.ArticleText != "" {
				fmt.Fprintf(b, "Article text:\n<<<\n%s\n>>>\n", c.ArticleText)
			}
		}
	}
}

func writeTaste(b *strings.Builder, taste string) {
	if taste == "" {
		return
	}
	b.WriteString("\n## The reader's taste (from their past reactions; adjust within the bar above, never beyond it)\n")
	b.WriteString(taste)
	b.WriteString("\n")
}

// BuildSelectPrompt asks for a shortlist of up to `shortlist` candidate IDs, judged from titles and excerpts.
func BuildSelectPrompt(cs []model.Candidate, taste string, shortlist int) string {
	var b strings.Builder
	b.WriteString(QualityBar)
	b.WriteString("\n")
	writeTaste(&b, taste)
	fmt.Fprintf(&b, "\n## Task\nFrom the candidates below, shortlist at most %d that most likely clear the bar, best first. "+
		"An empty list is fine.\nRespond with JSON only, no prose: {\"ids\": [\"c3\", \"c7\"]}\n\n## Candidates\n", shortlist)
	writeCandidates(&b, cs, false)
	return b.String()
}

// BuildWritePrompt asks for the final 0..maxItems write-ups from the shortlisted candidates.
func BuildWritePrompt(cs []model.Candidate, taste string, maxItems int) string {
	var b strings.Builder
	b.WriteString(QualityBar)
	b.WriteString("\n")
	writeTaste(&b, taste)
	fmt.Fprintf(&b, `
## Task
Read the shortlisted candidates below in full. Choose at most %d that clear the bar (zero is fine), best first, and write each up.

Rules:
- Base every statement only on the provided text. Do not invent facts.
- Every claim in summary, why_it_works and problem_solved carries a citation marker like [1].
- citations may only use that candidate's URL or its Discussion links.
- try_this is 2-4 concrete steps the reader could do today, not vague advice.
- hook is one line, at most 120 characters, saying why to open it.
- For paywalled candidates, use only the visible text and say less rather than guess.
- community_tldr: one sentence on what the Reddit discussion concluded, only if a Community TL;DR is given; else "".

Respond with JSON only, no prose, matching:
{"items": [{
  "candidate_id": "c1",
  "title": "...",
  "hook": "...",
  "summary": "... [1] ...",
  "try_this": ["...", "..."],
  "why_it_works": "... [1]",
  "problem_solved": "... [2]",
  "community_tldr": "",
  "citations": [{"n": 1, "url": "...", "label": "article"}, {"n": 2, "url": "...", "label": "HN discussion"}]
}]}

## Shortlisted candidates
`, maxItems)
	writeCandidates(&b, cs, true)
	return b.String()
}
