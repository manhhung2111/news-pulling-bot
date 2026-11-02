// Package model holds the types shared across the digest pipeline.
package model

import "time"

// Candidate is a post collected from a source, before the AI picks.
type Candidate struct {
	ID              string // stable within a run: "c1", "c2", ... assigned after merge
	Title           string
	URL             string // primary (original article when known)
	NormURL         string
	Discussion      []string // HN / Reddit permalinks
	Source          string   // source Name from config
	Published       time.Time
	Excerpt         string // ≤ 500 chars, plain text
	Points          int    // HN points, 0 otherwise
	Paywalled       bool
	RedditPermalink string // set for reddit candidates (TL;DR lookup)
	CommunityTLDR   string // filled in by enrich
	ArticleText     string // filled in by enrich (or by the source when the feed has the full text), ≤ 8000 chars
}

type Citation struct {
	N     int    `json:"n"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Item is one written-up pick in the digest.
type Item struct {
	CandidateID   string     `json:"candidate_id"`
	Title         string     `json:"title"`
	URL           string     `json:"url"`
	Source        string     `json:"source"`
	Hook          string     `json:"hook"`
	Summary       string     `json:"summary"` // contains [n] markers
	TryThis       []string   `json:"try_this"`
	WhyItWorks    string     `json:"why_it_works"`
	ProblemSolved string     `json:"problem_solved"`
	CommunityTLDR string     `json:"community_tldr,omitempty"`
	Discussion    []string   `json:"discussion,omitempty"`
	Citations     []Citation `json:"citations"`
	Paywalled     bool       `json:"-"` // copied from candidate
}

type SourceStatus struct {
	Name  string
	OK    bool
	Count int
	Err   string
}

type Digest struct {
	Date        time.Time // local date in config timezone
	Items       []Item
	Unavailable []string // source names with OK=false
	Provider    string   // "claude" | "gemini"
}
