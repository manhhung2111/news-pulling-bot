# AI Engineering Digest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go binary run daily by cron on EC2. It collects posts from configured sources, uses Claude Code (with Gemini as fallback) to pick and write up the best 0–5 items on applying AI coding agents across the SDLC, and posts them to Discord as a headline message plus a thread. It also learns from the owner's reactions and replies.

**Architecture:** A run-once pipeline: collect → merge/filter → sync feedback → AI shortlist → enrich (article text, Reddit TL;DR) → AI write → validate → post → record. Every external system sits behind a small interface (source HTTP via configurable base URLs, `ai.Provider`, `discord.API`), so the pipeline is tested with fakes. State lives in one SQLite file.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `github.com/mmcdole/gofeed`, `github.com/PuerkitoBio/goquery`, `github.com/go-shiori/go-readability`, `modernc.org/sqlite` (pure Go, no cgo), `github.com/bwmarrin/discordgo`, `google.golang.org/genai`, and the `claude` CLI.

**Spec:** `docs/superpowers/specs/2026-11-01-ai-eng-digest-design.md`

## Global Constraints

- Module path: `github.com/manhhung2111/news-pulling-bot`. Binary: `cmd/digest`.
- Schedule: daily 08:00 `Asia/Ho_Chi_Minh` (01:00 UTC). Cron line: `CRON_TZ=Asia/Ho_Chi_Minh` + `0 8 * * *`.
- Max items per digest: 5 (ceiling, not quota). 0 is valid.
- Lookback window default: 48h. Feedback window default: 14 days.
- A URL that has been sent is never sent again. A candidate shown to the AI is never shown again, unless the run failed before posting.
- Secrets come only from env: `DISCORD_BOT_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `GEMINI_API_KEY`.
- Only feedback from `discord.owner_user_id` counts.
- One source failing never stops the digest.
- Every URL in a posted item (primary, discussion, citations) must come from the candidate set. Items that break this are dropped.
- Discord limits: message content ≤ 2000 chars, embed description ≤ 4096, embed field value ≤ 1024, total embed chars ≤ 6000.
- Exact copy:
  - Headline: `☀️ <Mon, Jan 2> — N picks` (`1 pick` when N=1)
  - Zero-item headline: `☀️ <Mon, Jan 2> — Nothing met the bar today.`
  - Unavailable source note: `⚠️ <source name> unavailable today`
  - Fallback note: `_Made with fallback model (Gemini)._`
  - Failure message: `❌ Digest failed today: <reason>. Details are in the server log.`
  - Thread name: `Digest — <YYYY-MM-DD>`
  - Paywall label: `🔒 partly paywalled`
- Commits: one small commit per task (some tasks have two). Conventional-commit prefixes.

## Review Focus

1. **The first run on an empty DB floods with old posts.** Feeds return months of entries. The lookback filter must apply to every source, and undated entries are dropped (Task 5, Task 7).
2. **The same article arrives via HN, Reddit, and the original blog, with different URL forms** (utm params, trailing slash, `www.`, http vs https). It must become one candidate with discussion links attached (Task 3, Task 8).
3. **The AI returns malformed JSON, wraps it in a ```json fence, or invents/alters URLs.** Parse must tolerate fences. Validation must drop items with unknown URLs, not crash or post them (Task 10).
4. **An AI item's text overflows Discord limits** (long Try-this lists, many citations). Truncate with `…` instead of failing the post (Task 13).
5. **Claude hits its Pro usage limit, or `claude` isn't logged in on EC2.** Treat a non-zero exit or `is_error: true` as failure, fall back to Gemini, and if that fails too, post the failure line (Task 11, Task 16).

---

## File Structure

```
cmd/digest/main.go               flags, wiring, lock, exit codes
internal/config/config.go        YAML + env loading, defaults, validation
internal/model/model.go          Candidate, Item, Citation, Digest, SourceStatus
internal/urlnorm/urlnorm.go      URL normalization
internal/store/store.go          SQLite open + migrations
internal/store/queries.go        sent/seen/digests/items/feedback/kv
internal/sources/source.go       Source interface, Build(), CollectAll()
internal/sources/rss.go          generic RSS/Atom
internal/sources/hackernews.go   Algolia HN
internal/sources/reddit.go       subreddit RSS + TL;DR lookup
internal/sources/htmllisting.go  Anthropic-style listing pages
internal/pipeline/candidates.go  merge + filter
internal/enrich/article.go       readable article text
internal/ai/provider.go          Provider interface, Chain
internal/ai/claude.go            claude CLI provider
internal/ai/gemini.go            Gemini provider
internal/ai/prompt.go            quality bar, select + write prompts
internal/ai/parse.go             parse + validate AI output
internal/feedback/taste.go       taste notes + summary refresh
internal/discord/render.go       pure rendering to message/embeds
internal/discord/api.go          API interface + discordgo adapter
internal/discord/poster.go       post digest / failure
internal/discord/feedback.go     read reactions + owner replies
internal/pipeline/run.go         orchestration
config.example.yaml
deploy/digest.cron, deploy/digest.env.example, README.md
```

---

### Task 1: Project skeleton and config loading

**Files:**
- Create: `go.mod`, `.gitignore`, `internal/config/config.go`, `internal/config/config_test.go`, `config.example.yaml`, `cmd/digest/main.go` (stub)

**Interfaces:**
- Produces:
  ```go
  type Config struct {
      Timezone      string        `yaml:"timezone"`       // default "Asia/Ho_Chi_Minh"
      LookbackHours int           `yaml:"lookback_hours"` // default 48
      MaxItems      int           `yaml:"max_items"`      // default 5, must be 1..5
      ShortlistSize int           `yaml:"shortlist_size"` // default 8
      FeedbackDays  int           `yaml:"feedback_days"`  // default 14
      DBPath        string        `yaml:"db_path"`        // required
      UserAgent     string        `yaml:"user_agent"`     // default "ai-eng-digest/0.1"
      Discord       DiscordConfig `yaml:"discord"`        // ChannelID, OwnerUserID: required
      AI            AIConfig      `yaml:"ai"`             // ClaudeBin "claude", ClaudeTimeout 10m, GeminiModel "gemini-2.5-flash"
      Sources       []SourceConfig `yaml:"sources"`
      Secrets       Secrets       `yaml:"-"`
  }
  type SourceConfig struct {
      Name, Type, URL string; Enabled *bool          // nil = enabled
      LinkPrefix string `yaml:"link_prefix"`         // html_listing
      Keywords []string; MinPoints int `yaml:"min_points"` // hackernews
      Subreddit, Listing, Period string              // reddit_rss
      TLDRAuthor string `yaml:"tldr_author"`; TLDRMarker string `yaml:"tldr_marker"`
      Paywalled bool                                 // rss
  }
  type Secrets struct{ DiscordToken, ClaudeOAuthToken, GeminiAPIKey string }
  func Load(path string) (*Config, error)       // reads YAML, applies defaults, reads env, validates
  func (c *Config) Location() *time.Location
  ```

- [ ] **Step 1:** `git init`. Create `.gitignore` with `.superpowers/`, `*.db`, `*.db-*`, `/digest`, `.env`, `digest.env`. Run `go mod init github.com/manhhung2111/news-pulling-bot`, then `go get gopkg.in/yaml.v3`.
- [ ] **Step 2: Write failing tests** in `config_test.go`:
  - `TestLoadAppliesDefaults`: minimal YAML (db_path, discord ids, one rss source) → `LookbackHours==48`, `MaxItems==5`, `ShortlistSize==8`, `FeedbackDays==14`, `Timezone=="Asia/Ho_Chi_Minh"`, `AI.GeminiModel=="gemini-2.5-flash"`, `AI.ClaudeTimeout==10*time.Minute`.
  - `TestLoadReadsSecretsFromEnv`: `t.Setenv` the three vars → `Secrets` populated.
  - `TestLoadRejectsMissingRequired`: no `discord.channel_id` → error mentioning `channel_id`. Same for `owner_user_id` and `db_path`.
  - `TestLoadRejectsUnknownSourceType`: `type: foo` → error.
  - `TestLoadRejectsMaxItemsOver5`: `max_items: 6` → error.
- [ ] **Step 3:** Run `go test ./internal/config/...` → FAIL (undefined).
- [ ] **Step 4:** Implement `Load`. Known source types: `rss`, `hackernews`, `reddit_rss`, `html_listing`. Missing secrets are **not** an error here, because `--dry-run` doesn't need Discord.
- [ ] **Step 5:** Run tests → PASS. Write `config.example.yaml` with every starting source (see the appendix at the end). Write `cmd/digest/main.go` with `--config` and `--dry-run` flags that load the config and exit 0. Check that `go build ./...` succeeds.
- [ ] **Step 6: Commit** `chore: project skeleton and config loading`

---

### Task 2: Domain model

**Files:** Create `internal/model/model.go` (no tests; plain types)

**Interfaces:**
- Produces:
  ```go
  type Candidate struct {
      ID          string    // stable within a run: "c1", "c2", ... assigned after merge
      Title       string
      URL         string    // primary (original article when known)
      NormURL     string
      Discussion  []string  // HN / Reddit permalinks
      Source      string    // source Name from config
      Published   time.Time
      Excerpt     string    // ≤ 500 chars, plain text
      Points      int       // HN points, 0 otherwise
      Paywalled   bool
      RedditPermalink string // set for reddit candidates (TL;DR lookup)
      CommunityTLDR   string // filled in by enrich
      ArticleText     string // filled in by enrich, ≤ 8000 chars
  }
  type Citation struct { N int `json:"n"`; URL string `json:"url"`; Label string `json:"label"` }
  type Item struct {
      CandidateID   string     `json:"candidate_id"`
      Title         string     `json:"title"`
      URL           string     `json:"url"`
      Source        string     `json:"source"`
      Hook          string     `json:"hook"`
      Summary       string     `json:"summary"`        // contains [n] markers
      TryThis       []string   `json:"try_this"`
      WhyItWorks    string     `json:"why_it_works"`
      ProblemSolved string     `json:"problem_solved"`
      CommunityTLDR string     `json:"community_tldr,omitempty"`
      Discussion    []string   `json:"discussion,omitempty"`
      Citations     []Citation `json:"citations"`
      Paywalled     bool       `json:"-"`              // copied from candidate
  }
  type SourceStatus struct { Name string; OK bool; Count int; Err string }
  type Digest struct {
      Date        time.Time      // local date in config timezone
      Items       []Item
      Unavailable []string       // source names with OK=false
      Provider    string         // "claude" | "gemini"
  }
  ```
- [ ] **Step 1:** Write the file. Run `go build ./...` → OK.
- [ ] **Step 2: Commit** `feat: domain model types`

---

### Task 3: URL normalization

**Files:** Create `internal/urlnorm/urlnorm.go`, `internal/urlnorm/urlnorm_test.go`

**Interfaces:** Produces `func Normalize(raw string) string`. It returns `""` for unparseable or non-http(s) input.

- [ ] **Step 1: Write table test `TestNormalize`.** Each pair must map to the same output:
  - `https://www.example.com/post/` ↔ `http://example.com/post`
  - `https://example.com/a?utm_source=x&utm_medium=y&id=3` ↔ `https://example.com/a?id=3`
  - `https://example.com/a?b=2&a=1` ↔ `https://example.com/a?a=1&b=2`
  - `https://example.com/a#section` ↔ `https://example.com/a`
  - `https://EXAMPLE.com/A` → host lowercased, path case kept: `https://example.com/A`
  - `https://news.ycombinator.com/item?id=1` keeps `id`
  - `mailto:x@y.z` → `""`. `""` → `""`.
  - Also strip `ref`, `fbclid`, `gclid`, and any `utm_*`.
- [ ] **Step 2:** `go test ./internal/urlnorm/...` → FAIL.
- [ ] **Step 3:** Implement. Output scheme is always `https`. Drop the `www.` prefix and the fragment. Sort the query. Trim the trailing `/` except at the root.
- [ ] **Step 4:** Tests PASS.
- [ ] **Step 5: Commit** `feat: URL normalization for dedupe`

---

### Task 4: SQLite store

**Files:** Create `internal/store/store.go`, `internal/store/queries.go`, `internal/store/store_test.go`

**Interfaces:**
- Produces:
  ```go
  type Store struct{ /* *sql.DB */ }
  func Open(path string) (*Store, error)   // creates file + runs migrations (idempotent)
  func (s *Store) Close() error
  func (s *Store) IsSent(normURL string) (bool, error)
  func (s *Store) IsSeen(normURL string) (bool, error)
  func (s *Store) MarkSeen(normURLs []string, at time.Time) error
  type DigestRecord struct{ Date string; ChannelID, MessageID, ThreadID, Provider string; PostedAt time.Time }
  type ItemRecord struct{ MessageID, DigestDate, NormURL, Title, Source string }
  func (s *Store) RecordDigest(d DigestRecord, items []ItemRecord) error // one tx; also inserts each item's NormURL into sent
  func (s *Store) RecentDigests(since time.Time) ([]DigestRecord, error)
  func (s *Store) ItemsForDigest(date string) ([]ItemRecord, error)
  type Feedback struct{ Key, DigestDate, ItemMessageID, Kind, Text string; CreatedAt time.Time } // Kind: "up"|"down"|"comment"
  func (s *Store) UpsertFeedback(f []Feedback) error                   // INSERT OR IGNORE on Key
  func (s *Store) FeedbackSince(since time.Time) ([]FeedbackWithItem, error) // joined with digest_items for Title/Source
  func (s *Store) FeedbackBefore(before time.Time, after time.Time) ([]FeedbackWithItem, error)
  type FeedbackWithItem struct{ Feedback; Title, Source string }
  func (s *Store) GetKV(key string) (string, bool, error)
  func (s *Store) SetKV(key, value string) error
  ```
- Tables:
  - `sent(norm_url PK, digest_date, sent_at)`
  - `seen(norm_url PK, first_seen_at)`
  - `digests(date PK, channel_id, message_id, thread_id, provider, posted_at)`
  - `digest_items(message_id PK, digest_date, norm_url, title, source)`
  - `feedback(key PK, digest_date, item_message_id, kind, text, created_at)`
  - `kv(key PK, value)`

- [ ] **Step 1: Write failing tests** (each uses `filepath.Join(t.TempDir(), "t.db")`):
  - `TestOpenIsIdempotent`: Open, Close, Open again → no error.
  - `TestRecordDigestMarksSent`: after `RecordDigest` with one item, `IsSent(norm)==true`.
  - `TestMarkSeen`: `IsSeen` false → `MarkSeen` → true. Marking the same URL twice is no error.
  - `TestUpsertFeedbackDedupesByKey`: insert the same `Key` twice → `FeedbackSince` returns 1 row, with the `Title` from the joined item.
  - `TestRecentDigestsFiltersByTime`.
  - `TestKV`: missing → `ok=false`. Set, then get, round-trips.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** `go get modernc.org/sqlite`. Implement with driver name `sqlite`. Migrations use `CREATE TABLE IF NOT EXISTS`. Set `PRAGMA journal_mode=WAL`.
- [ ] **Step 4:** Tests PASS.
- [ ] **Step 5: Commit** `feat: sqlite store for history, digests and feedback`

---

### Task 5: Source interface and generic RSS source

**Files:** Create `internal/sources/source.go`, `internal/sources/rss.go`, `internal/sources/rss_test.go`, `internal/sources/testdata/feed.atom`

**Interfaces:**
- Produces:
  ```go
  type Source interface {
      Name() string
      Fetch(ctx context.Context, since time.Time) ([]model.Candidate, error)
  }
  type Deps struct{ HTTP *http.Client; UserAgent string }
  func Build(cfgs []config.SourceConfig, d Deps) ([]Source, error) // skips Enabled==false
  func CollectAll(ctx context.Context, srcs []Source, since time.Time) ([]model.Candidate, []model.SourceStatus)
  func NewRSS(cfg config.SourceConfig, d Deps) Source
  ```
- Every source sets `Candidate.NormURL = urlnorm.Normalize(URL)`. It drops entries with no parseable date or with a date before `since`, and trims `Excerpt` to plain text ≤ 500 chars.

- [ ] **Step 1: Write failing tests.** Serve `testdata/feed.atom` from `httptest.Server`. It has 3 entries: one 1h old, one 72h old, and one with no date. Use the dates relative to a fixed `since`, generating the fixture times in the test with a template.
  - `TestRSSFiltersBySinceAndUndated`: since = now-48h → exactly 1 candidate, with Title, URL, Source==cfg.Name, and Excerpt stripped of HTML.
  - `TestRSSSetsPaywalled`: cfg.Paywalled=true → candidate.Paywalled.
  - `TestRSSSendsUserAgent`: the server asserts the `User-Agent` header equals Deps.UserAgent.
  - `TestCollectAllIsolatesFailures`: one fake source returns an error and one returns 2 candidates → 2 candidates, and statuses `[{A,false,0,"..."},{B,true,2,""}]` in source order.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** `go get github.com/mmcdole/gofeed`. Implement. Use `PublishedParsed`, falling back to `UpdatedParsed`. `CollectAll` runs sources concurrently with a per-source 30s timeout, and keeps the output order stable by source index.
- [ ] **Step 4:** Tests PASS.
- [ ] **Step 5: Commit** `feat: source interface and generic RSS source`

---

### Task 6: Hacker News source

**Files:** Create `internal/sources/hackernews.go`, `internal/sources/hackernews_test.go`

**Interfaces:** `func NewHackerNews(cfg config.SourceConfig, d Deps, baseURL string) Source`. `Build` passes `https://hn.algolia.com`.

- Request: `GET {base}/api/v1/search_by_date?tags=story&hitsPerPage=200&numericFilters=created_at_i>{since.Unix()},points>={MinPoints}`.
- Keep a hit if its title contains any keyword as a case-insensitive whole word (`AI` must not match `said`).
- Discussion link: `https://news.ycombinator.com/item?id={objectID}`. If `url` is empty (Ask HN), `URL` = discussion and `Discussion` is empty. `Points` = `points`. Excerpt = `story_text` stripped.

- [ ] **Step 1: Write failing tests** with an httptest server returning 4 hits:
  - "Using Claude Code for PR review" (url set) → kept
  - "He said hello" → dropped (the `AI` keyword must not match)
  - "Ask HN: agents in CI?" (no url) → kept, URL = item link
  - "Rust 2.0 released" → dropped

  Tests: `TestHackerNewsKeywordFilterWholeWord`, `TestHackerNewsAskHNUsesItemURL`, and `TestHackerNewsQueryParams` (the server asserts `numericFilters` contains `points>=30` when MinPoints=30).
- [ ] **Step 2:** FAIL → **Step 3:** implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: Hacker News source via Algolia API`

---

### Task 7: Reddit RSS source with TL;DR lookup

**Files:** Create `internal/sources/reddit.go`, `internal/sources/reddit_test.go`, `internal/sources/testdata/reddit_listing.atom`, `internal/sources/testdata/reddit_comments.atom`

**Interfaces:**
- `func NewReddit(cfg config.SourceConfig, d Deps, baseURL string) *Reddit`. Base is `https://www.reddit.com`. It implements `Source`.
- `func (r *Reddit) FetchTLDR(ctx context.Context, permalink string) (string, error)`. It fetches `{permalink}.rss` and returns the text of the first entry whose author is `/u/{TLDRAuthor}` (when set) or whose content contains `TLDRMarker`. Returns `""` if none. Text is stripped to plain text, ≤ 600 chars.
- Listing URL: `{base}/r/{Subreddit}/{Listing}/.rss?t={Period}`. Defaults: listing `top`, period `day`.
- Link posts: the entry content holds `<a href="X">[link]</a>`. If X is not a reddit.com URL, `URL`=X and `Discussion=[permalink]`. Self posts: `URL`=permalink. `RedditPermalink` = permalink in both cases.
- Non-200 response → error. `CollectAll` reports the source as unavailable.

- [ ] **Step 1: Write failing tests:**
  - `TestRedditLinkPostUsesExternalURL`
  - `TestRedditSelfPostUsesPermalink`
  - `TestRedditBlockedReturnsError`: server returns 403 → `Fetch` error.
  - `TestRedditTLDRByAuthor` and `TestRedditTLDRByMarker`
  - `TestRedditTLDRNoneFound` → `""`
- [ ] **Step 2:** FAIL → **Step 3:** implement (gofeed) → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: Reddit RSS source with community TL;DR lookup`

---

### Task 8: HTML listing source (Anthropic) and wire up Build

**Files:** Create `internal/sources/htmllisting.go`, `internal/sources/htmllisting_test.go`. Modify `internal/sources/source.go` (`Build` switch).

**Interfaces:** `func NewHTMLListing(cfg config.SourceConfig, d Deps) Source`

- Fetch `cfg.URL` and collect unique `<a href>` values whose path starts with `cfg.LinkPrefix` (and is longer than it), resolved against `cfg.URL`. Take the first 15.
- For each, fetch the page. The published date is the first one found of: `meta[property="article:published_time"]`, JSON-LD `datePublished`, `time[datetime]`. Title: `og:title`, falling back to `<title>`. Excerpt: `meta[name=description]`.
- Drop pages with no date or a date before `since`. A failing page fetch skips that page; only a failing listing fetch fails the source.

- [ ] **Step 1: Write failing tests.** Serve a listing with 3 links (one under the prefix twice, one outside the prefix, one more under the prefix) and two article pages: one dated 2h ago via `article:published_time`, and one with only `time[datetime]` dated 5 days ago.
  - `TestHTMLListingKeepsRecentPrefixedLinks` → 1 candidate, with the title from og:title.
  - `TestHTMLListingDedupesLinks` → the article page is fetched once (count hits on the server).
  - `TestHTMLListingSkipsBrokenArticle` → an article returning 500 doesn't fail the source.
- [ ] **Step 2:** FAIL → **Step 3:** `go get github.com/PuerkitoBio/goquery`, implement, and fill in the `Build` switch for all four types. Add `TestBuildAllTypes`: one config entry of each of the four types plus one extra entry with `enabled: false` → 4 sources, with the disabled one skipped. → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: HTML listing source for Anthropic blog; wire all sources`

---

### Task 9: Merge and filter candidates

**Files:** Create `internal/pipeline/candidates.go`, `internal/pipeline/candidates_test.go`

**Interfaces:**
- ```go
  type SeenChecker interface{ IsSent(string) (bool, error); IsSeen(string) (bool, error) }
  func MergeCandidates(cs []model.Candidate) []model.Candidate
  func FilterNew(cs []model.Candidate, st SeenChecker) ([]model.Candidate, error)
  func AssignIDs(cs []model.Candidate)  // "c1".."cN" in order
  ```
- **Merge:** group by `NormURL`. The group's primary is the non-HN, non-Reddit candidate when present (the original article), else the first. Take the union of `Discussion` (normalized-deduped), the max of `Points`, the first non-empty `RedditPermalink`, and the earliest `Published`. Output is sorted by `Published` descending. Candidates with empty `NormURL` are dropped.
- **Filter:** drop a candidate if its `NormURL` or any of its discussion URLs is sent, or if its `NormURL` is seen.

- [ ] **Step 1: Write failing tests:**
  - `TestMergeCollapsesSameArticle`: the same post from "Anthropic News", "Hacker News" (with a utm param), and "r/ClaudeAI" → 1 candidate, with Source "Anthropic News", 2 discussion links, and Points from HN.
  - `TestMergeKeepsDistinct`.
  - `TestFilterDropsSentAndSeen`: a fake checker with one sent URL and one seen URL → 1 left.
  - `TestFilterDropsWhenDiscussionAlreadySent`.
  - `TestAssignIDs`.
- [ ] **Step 2:** FAIL → **Step 3:** implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: merge duplicate candidates and filter already-sent`

---

### Task 10: Prompts, parsing and validation

**Files:** Create `internal/ai/prompt.go`, `internal/ai/parse.go`, `internal/ai/prompt_test.go`, `internal/ai/parse_test.go`

**Interfaces:**
- ```go
  const QualityBar = `...`  // the exact text is below
  func BuildSelectPrompt(cs []model.Candidate, taste string, shortlist int) string
  func BuildWritePrompt(cs []model.Candidate, taste string, maxItems int) string
  func ParseShortlist(raw string, valid map[string]bool) ([]string, error) // expects {"ids":["c3",...]}; drops unknown ids, dedupes, keeps order
  func ParseItems(raw string) ([]model.Item, error)                     // expects {"items":[...]}
  func ValidateItems(items []model.Item, byID map[string]model.Candidate, maxItems int) []model.Item
  ```
- `QualityBar` must state the spec's bar (§6.4) in plain words. Include, verbatim in meaning:
  - Pick only items teaching a concrete technique an engineer could apply, either to their coding-agent setup (tools, context, knowledge, skills or workflows, memory, decision logs) or to using agents at a stage of the SDLC (specs, design, review, testing, CI/CD, incidents, docs, migrations, team process).
  - Reject hype, opinion-only pieces, launch announcements without practices, beginner content, and general software engineering with no AI angle. Popularity is not quality.
  - Fewer is better than weak. Returning zero items is correct when nothing qualifies.
  - Never pad.
- The write prompt includes each shortlisted candidate's ID, title, URL, discussion links, source, excerpt, `ArticleText`, and `CommunityTLDR`. It requires:
  - every claim in summary / why_it_works / problem_solved to carry an `[n]` marker
  - `citations` URLs only from that candidate's URL and discussion links
  - `try_this` as 2–4 concrete steps
  - `hook` ≤ 120 chars
  - for paywalled candidates, use only the visible text
  - output JSON only, matching the schema embedded in the prompt
- `Parse*` strips an optional leading/trailing ```` ``` ```` / ```` ```json ```` fence, then `json.Unmarshal`.
- **Validate:**
  - Drop items whose `CandidateID` is unknown.
  - Overwrite `URL`, `Source`, `Discussion`, and `Paywalled` from the candidate (never trust the AI's versions).
  - Drop citations whose normalized URL is not the candidate's `NormURL` or a normalized discussion URL.
  - Drop items with an empty Summary, an empty TryThis, or zero valid citations.
  - Set `CommunityTLDR` from the candidate when the AI left it empty.
  - Truncate to `maxItems`.

- [ ] **Step 1: Write failing tests:**
  - `TestBuildSelectPromptContainsBarAndIDs`
  - `TestBuildWritePromptIncludesArticleTextAndTLDR`
  - `TestParseShortlistStripsFenceAndUnknownIDs`
  - `TestParseItemsInvalidJSONErrors`
  - `TestValidateDropsUnknownCandidate`
  - `TestValidateOverwritesURLFromCandidate`
  - `TestValidateDropsForeignCitations`
  - `TestValidateDropsItemWithNoCitations`
  - `TestValidateTruncatesToMax`
- [ ] **Step 2:** FAIL → **Step 3:** implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: AI prompts and output validation`

---

### Task 11: AI providers — Claude CLI and fallback chain

**Files:** Create `internal/ai/provider.go`, `internal/ai/claude.go`, `internal/ai/claude_test.go`, `internal/ai/provider_test.go`

**Interfaces:**
- ```go
  type Provider interface{ Name() string; Complete(ctx context.Context, prompt string) (string, error) }
  type Chain struct{ Providers []Provider }
  // Complete tries each provider in order; returns the text and the name of the provider that succeeded.
  func (c Chain) Complete(ctx context.Context, prompt string) (text, provider string, err error)
  func NewClaude(bin string, timeout time.Duration, oauthToken string) Provider // Name() "claude"
  ```
- Claude:
  - Run `{bin} -p --output-format json`, with the prompt on stdin and env plus `CLAUDE_CODE_OAUTH_TOKEN` (when non-empty), under the timeout.
  - Parse stdout as `{"is_error":bool,"result":string}`.
  - Non-zero exit, `is_error:true`, empty `result`, or unparseable stdout → error, with stderr's first 300 chars included.

- [ ] **Step 1: Write failing tests:**
  - Claude tests write a fake `claude` shell script into `t.TempDir()` and pass its path as `bin`:
    - `TestClaudeReturnsResult`: the script echoes `{"is_error":false,"result":"hi"}` → `"hi"`.
    - `TestClaudePassesPromptOnStdin`: the script writes stdin to a file → the file equals the prompt.
    - `TestClaudeIsErrorFails`
    - `TestClaudeNonZeroExitFails`: the stderr text appears in the error.
    - `TestClaudeTimeout`: the script sleeps 5s, timeout 100ms → error.
    - `TestClaudePassesOAuthEnv`
  - Chain tests use fake providers:
    - `TestChainFallsBack`: first errors, second returns → provider name is the second's.
    - `TestChainAllFail`: the error mentions both names.
- [ ] **Step 2:** FAIL → **Step 3:** implement (`os/exec`, `exec.CommandContext`) → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: Claude Code CLI provider and fallback chain`

---

### Task 12: Gemini provider

**Files:** Create `internal/ai/gemini.go`, `internal/ai/gemini_test.go`

**Interfaces:**
- ```go
  type GenerateFunc func(ctx context.Context, model, prompt string) (string, error)
  func NewGemini(apiKey, model string) (Provider, error)          // real client via google.golang.org/genai; Name() "gemini"
  func newGeminiWith(gen GenerateFunc, model string, backoff []time.Duration) Provider // for tests
  ```
- Request JSON output (`ResponseMIMEType: "application/json"`). Retry on errors that look like rate limits or 5xx (HTTP 429/500/503 or a message containing `RESOURCE_EXHAUSTED`), with backoff `2s, 8s, 30s`. Return other errors immediately. Empty `apiKey` → `NewGemini` returns an error, and the run uses Claude only.

- [ ] **Step 1: Write failing tests** with a fake `GenerateFunc` and backoff `[1ms,1ms,1ms]`:
  - `TestGeminiRetriesOn429ThenSucceeds`: 2 failures, then OK → 3 calls.
  - `TestGeminiGivesUpAfterRetries`: 4 calls → error.
  - `TestGeminiNoRetryOn400`: 1 call.
- [ ] **Step 2:** FAIL → **Step 3:** `go get google.golang.org/genai`, implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: Gemini fallback provider with backoff`

---

### Task 13: Discord rendering (pure)

**Files:** Create `internal/discord/render.go`, `internal/discord/render_test.go`

**Interfaces:**
- ```go
  type Rendered struct {
      Headline   string                     // ≤ 2000 chars
      ThreadName string
      Items      []*discordgo.MessageEmbed  // one per item, each ≤ 6000 total chars
  }
  func Render(d model.Digest) Rendered
  func RenderFailure(reason string) string  // reason trimmed to 300 chars
  ```
- **Headline:** the copy from Global Constraints, then for each item `N. [Title](<URL>) — Source` followed by a new line `   Hook`. Then a blank line, one `⚠️ … unavailable today` line per unavailable source, and the fallback note when `Provider=="gemini"`. If the result exceeds 2000 chars, shorten hooks first, then titles.
- **Embed per item:**
  - Title (≤256) and URL.
  - Description = Summary, with `🔒 partly paywalled` on its own first line when Paywalled.
  - Fields: `✅ Try this` (bulleted `• `), `💡 Why it works`, `🎯 Problem it solves`, `💬 Community TL;DR` (only when set), `🔗 Discussion` (only when set), `📚 Sources` (`[n] [label](url)` per line).
  - Footer = Source. Color `0xD97757`.
  - Each field ≤1024 and description ≤4096, truncated with `…`. If the total still exceeds 6000, shorten Summary.
- `ThreadName` = `Digest — 2026-11-01`. Date format uses Go layout `Mon, Jan 2`.

- [ ] **Step 1: Write failing tests:**
  - `TestRenderHeadlineThreePicks`: the exact string for a fixed 3-item digest dated 2026-11-01 begins `☀️ Sun, Nov 1 — 3 picks`.
  - `TestRenderHeadlineOnePick` → `1 pick`.
  - `TestRenderZeroItems`: exact headline `☀️ Sun, Nov 1 — Nothing met the bar today.`, and `Items` is empty.
  - `TestRenderUnavailableAndFallbackNotes`
  - `TestRenderEmbedFieldsAndOrder`
  - `TestRenderPaywallLabel`
  - `TestRenderTruncatesLongFields`: a 5000-char Try-this → every field ≤1024, and the total ≤6000.
  - `TestRenderHeadlineUnder2000`: 5 items with 600-char hooks.
  - `TestRenderFailure`
- [ ] **Step 2:** FAIL → **Step 3:** `go get github.com/bwmarrin/discordgo`, implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: render digest into Discord headline and embeds`

---

### Task 14: Discord posting

**Files:** Create `internal/discord/api.go`, `internal/discord/poster.go`, `internal/discord/poster_test.go`

**Interfaces:**
- ```go
  type API interface {
      Send(channelID, content string) (msgID string, err error)            // suppresses link embeds
      StartThread(channelID, msgID, name string) (threadID string, err error) // auto-archive 1440 min
      SendEmbed(channelID string, e *discordgo.MessageEmbed) (msgID string, err error)
      React(channelID, msgID, emoji string) error
      Reactions(channelID, msgID, emoji string) (userIDs []string, err error)
      Messages(channelID string, limit int) ([]Message, error)
  }
  type Message struct{ ID, AuthorID, Content, ReplyToID string; Timestamp time.Time }
  func NewAPI(token string) (API, error)     // discordgo.New("Bot "+token) adapter; no gateway connection
  type Posted struct{ MessageID, ThreadID string; ItemMessageIDs []string } // ItemMessageIDs[i] ↔ Digest.Items[i]
  func PostDigest(api API, channelID string, r Rendered) (Posted, error)
  func PostFailure(api API, channelID, reason string) error
  ```
- `PostDigest`: send the headline. If there are items: start a thread, send each embed into it, and react `👍` then `👎` on each. Zero items → no thread (`ThreadID==""`). A failed reaction is logged and ignored. A failed headline/thread/embed send is returned as an error.

- [ ] **Step 1: Write failing tests** with a recording fake API:
  - `TestPostDigestOrderOfCalls`: Send, StartThread, then SendEmbed+React×2 per item, with the thread name passed through.
  - `TestPostDigestZeroItemsNoThread`
  - `TestPostDigestReactionFailureIgnored`
  - `TestPostFailureSendsRenderedText`
- [ ] **Step 2:** FAIL → **Step 3:** implement the poster. Also implement the discordgo adapter. It has no unit test; it gets exercised manually in Task 17. `Send` sets `Flags: discordgo.MessageFlagsSuppressEmbeds`. → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: post digest headline, thread and reactions to Discord`

---

### Task 15: Feedback sync and taste notes

**Files:** Create `internal/discord/feedback.go`, `internal/discord/feedback_test.go`, `internal/feedback/taste.go`, `internal/feedback/taste_test.go`

**Interfaces:**
- ```go
  // discord/feedback.go
  type FeedbackStore interface {
      RecentDigests(since time.Time) ([]store.DigestRecord, error)
      ItemsForDigest(date string) ([]store.ItemRecord, error)
      UpsertFeedback([]store.Feedback) error
  }
  func SyncFeedback(api API, st FeedbackStore, ownerID string, since time.Time) error
  // feedback/taste.go
  type TasteStore interface {
      FeedbackSince(time.Time) ([]store.FeedbackWithItem, error)
      FeedbackBefore(before, after time.Time) ([]store.FeedbackWithItem, error)
      GetKV(string) (string, bool, error); SetKV(string, string) error
  }
  func TasteNotes(st TasteStore, now time.Time, windowDays int) (string, error)
  type CompleteFunc func(ctx context.Context, prompt string) (string, error) // pipeline passes a wrapper around ai.Chain.Complete
  func RefreshSummary(ctx context.Context, st TasteStore, complete CompleteFunc, now time.Time, windowDays int) error
  ```
- **SyncFeedback:** for each digest since `since` that has a thread:
  - For each item message, read `👍`/`👎` reactions. When the owner is among them, write `Key="react:{msgID}:up|down"`, `Kind` up/down.
  - Read reactions from the item messages inside the digest's thread (`channelID` = `ThreadID`).
  - Read thread messages (limit 100). Each message by the owner becomes `Key="reply:{id}"`, `Kind="comment"`, `ItemMessageID` = `ReplyToID` when it matches an item, else `""` (thread-level).
  - Messages from other users are ignored.
  - API errors on one digest are logged and skipped.
- **TasteNotes:** returns `""` when there is no feedback and no summary. Otherwise:
  ```
  Owner's standing preferences: <kv taste_summary, if any>
  Recent feedback (newest first):
  - 👍 "<title>" (<source>)
  - 👎 "<title>" (<source>)
  - 💬 on "<title>": <text>      (or: 💬 general: <text>)
  ```
  Capped at 40 lines.
- **RefreshSummary:**
  - If kv `taste_summary_updated_at` is less than 7 days old, do nothing.
  - Otherwise take feedback created before `now-windowDays` and after kv `taste_folded_until`. If there is any, ask the provider to merge it into the existing summary, ≤ 120 words, focused on what the owner wants more or less of.
  - Store `taste_summary`, `taste_summary_updated_at`, and `taste_folded_until=now-windowDays`.
  - A provider error is logged by the caller and does not fail the run.

- [ ] **Step 1: Write failing tests:**
  - `TestSyncFeedbackOwnerOnly`: reactions from owner and other user → only the owner's are stored.
  - `TestSyncFeedbackReplyMapsToItem`
  - `TestSyncFeedbackThreadLevelComment`
  - `TestTasteNotesEmpty`
  - `TestTasteNotesFormat`
  - `TestRefreshSummarySkipsWhenFresh`
  - `TestRefreshSummaryFoldsOldFeedback`: the fake provider receives the old feedback text, and kv is updated.
- [ ] **Step 2:** FAIL → **Step 3:** implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: sync owner feedback from Discord and build taste notes`

---

### Task 16: Pipeline orchestration and article enrichment

**Files:** Create `internal/enrich/article.go`, `internal/enrich/article_test.go`, `internal/pipeline/run.go`, `internal/pipeline/run_test.go`

**Interfaces:**
- ```go
  // enrich
  func ArticleText(ctx context.Context, hc *http.Client, ua, url string) (string, error) // go-readability; plain text ≤ 8000 chars
  // pipeline
  type TLDRFetcher interface{ FetchTLDR(ctx context.Context, permalink string) (string, error) }
  type Deps struct {
      Cfg      *config.Config
      Store    *store.Store
      Sources  []sources.Source
      TLDR     TLDRFetcher        // nil when no reddit source
      AI       ai.Chain
      Discord  discord.API        // nil in dry-run
      Article  func(ctx context.Context, url string) (string, error)
      Now      func() time.Time
      Out      io.Writer          // dry-run output
      Log      *slog.Logger
  }
  func Run(ctx context.Context, d Deps, dryRun bool) error
  ```
- **Run order:**
  1. `since = Now() - LookbackHours`. Then `CollectAll`, `MergeCandidates`, `FilterNew`, `AssignIDs`.
  2. If not dry-run: `SyncFeedback(since = now - FeedbackDays)`, then `RefreshSummary` (errors are logged). Then `TasteNotes`.
  3. If there are 0 candidates, skip the AI. Otherwise:
     - Select prompt → `Chain.Complete` → `ParseShortlist`.
     - For each shortlisted candidate: `ArticleText` (errors leave it empty), and a TL;DR for those with a `RedditPermalink`.
     - Write prompt → `Chain.Complete` → `ParseItems` → `ValidateItems`.
     - Parse failure on write → retry the write once with the same chain, then fail.
     - The provider recorded is the one used for the write step.
  4. Build `model.Digest{Date: Now() in cfg.Location(), Unavailable: names with OK=false}`, then `Render`.
  5. **Dry-run:** print the headline and each embed as readable text to `Out`. Don't touch the store's sent/seen tables or Discord.
  6. **Otherwise:** `PostDigest`, then `RecordDigest` (date `YYYY-MM-DD`, items mapped to `ItemMessageIDs`), then `MarkSeen` (all candidate NormURLs offered to the select step).
  7. **On any error before PostDigest succeeds** (not dry-run): `PostFailure(reason)`, return the error, and don't `MarkSeen`.
  8. If all sources fail, the run still posts a zero-item digest with the unavailable notes. That's not a failure.

- [ ] **Step 1: Write failing tests** for `ArticleText` (an httptest page with a nav and an article body → the text has the body and not the nav; output ≤ 8000) and for `Run`, using fake sources, a fake chain, a fake Discord API, and a temp-file store:
  - `TestRunHappyPath`: 2 candidates → digest posted, `IsSent` true for the picked URL, and `IsSeen` true for both.
  - `TestRunDryRunWritesOutputOnly`: `Out` contains the headline, Discord has no calls, and `IsSeen` is false.
  - `TestRunNoCandidatesPostsNothingMetTheBar`: the AI is not called.
  - `TestRunAIFailurePostsFailureAndDoesNotMarkSeen`
  - `TestRunUnavailableSourceNoted`
  - `TestRunUsesFallbackProviderName`: the first provider fails → the headline has the fallback note.
  - `TestRunSecondDayDoesNotReofferSeen`: two runs on the same candidates → the second run's select prompt contains no candidates and posts "Nothing met the bar today."
- [ ] **Step 2:** FAIL → **Step 3:** `go get github.com/go-shiori/go-readability`, implement → **Step 4:** PASS.
- [ ] **Step 5: Commit** `feat: enrich shortlisted articles with readable text` (enrich only), then **commit** `feat: pipeline orchestration with dry-run and failure posting`.

---

### Task 17: CLI wiring, lock, deploy files, README

**Files:** Modify `cmd/digest/main.go`. Create `cmd/digest/lock.go`, `cmd/digest/lock_test.go`, `deploy/digest.cron`, `deploy/digest.env.example`, `README.md`.

**Interfaces:** `func acquireLock(path string) (release func(), err error)`. A non-blocking `syscall.Flock` on `path`. Returns an error when another process holds the lock.

- `main` does the following:
  - Load config and open the store.
  - Build sources (passing the `*Reddit` as `TLDR` when present) and the chain `[NewClaude, NewGemini if key set]`.
  - Build the Discord API (skipped in dry-run; required otherwise → exit 2 with a clear message if `DISCORD_BOT_TOKEN` is missing).
  - Take the lock at `DBPath + ".lock"`, run, and log one summary line via `slog`: `candidates_per_source`, `picked`, `provider`, `error`.
  - Exit 1 on run error and 0 on success.
- `deploy/digest.cron`:
  ```
  CRON_TZ=Asia/Ho_Chi_Minh
  0 8 * * * . /etc/ai-eng-digest/digest.env && /usr/local/bin/digest --config /etc/ai-eng-digest/config.yaml >> /var/log/ai-eng-digest.log 2>&1
  ```
- `deploy/digest.env.example` holds `export` lines for the three secrets.
- `README.md` is a step-by-step setup guide:
  1. Build: `GOOS=linux GOARCH=amd64 go build -o digest ./cmd/digest` (use arm64 for Graviton). Copy to EC2.
  2. Install the `claude` CLI on EC2. Run `claude setup-token` and put the token in the env file.
  3. Create the Discord app and bot. Enable **Message Content Intent**. Invite with Send Messages, Create Public Threads, Send Messages in Threads, Read Message History, and Add Reactions. Copy the channel ID and your user ID (Developer Mode).
  4. Get a Gemini key (optional).
  5. Copy and edit the config, then run `--dry-run` first.
  6. Check Reddit reachability with `curl -s -o /dev/null -w "%{http_code}\n" -A "ai-eng-digest/0.1" https://www.reddit.com/r/ClaudeAI/.rss`.
  7. Confirm the r/ClaudeAI TL;DR bot's username on a live thread and set `tldr_author`.
  8. Install the crontab.

- [ ] **Step 1: Write failing test** `TestLockExclusive`: first acquire OK, second → error, and after release a third acquire succeeds.
- [ ] **Step 2:** FAIL → **Step 3:** implement the lock and main wiring → **Step 4:** `go test ./...` all PASS. `go vet ./...` clean. `go build ./cmd/digest` OK.
- [ ] **Step 5: Manual smoke test** (owner's machine, real network, no Discord): `go run ./cmd/digest --config config.example.yaml --dry-run`. Expected: per-source counts logged, and a readable digest (or "Nothing met the bar today.") printed. Record anything surprising (for example, Anthropic page markup) as a follow-up.
- [ ] **Step 6: Commit** `feat: CLI wiring with run lock` then **commit** `docs: deploy files and setup README`.

---

## Appendix: `config.example.yaml` starting sources

```yaml
timezone: Asia/Ho_Chi_Minh
lookback_hours: 48
max_items: 5
shortlist_size: 8
feedback_days: 14
db_path: ./digest.db
user_agent: "ai-eng-digest/0.1 (personal digest bot)"
discord:
  channel_id: "REPLACE_ME"
  owner_user_id: "REPLACE_ME"
ai:
  claude_bin: claude
  claude_timeout: 10m
  gemini_model: gemini-2.5-flash
sources:
  - {name: Anthropic News, type: html_listing, url: "https://www.anthropic.com/news", link_prefix: /news/}
  - {name: Anthropic Engineering, type: html_listing, url: "https://www.anthropic.com/engineering", link_prefix: /engineering/}
  - name: Hacker News
    type: hackernews
    min_points: 30
    keywords: [Claude, Anthropic, agent, agents, agentic, LLM, LLMs, AI, MCP, Cursor, Copilot, Codex, prompt, context, skill, skills, RAG]
  - {name: r/ClaudeAI, type: reddit_rss, subreddit: ClaudeAI, listing: top, period: day, tldr_author: "", tldr_marker: "TL;DR"}
  - {name: Simon Willison, type: rss, url: "https://simonwillison.net/atom/entries/"}
  - {name: Martin Fowler, type: rss, url: "https://martinfowler.com/feed.atom"}
  - {name: The Pragmatic Engineer, type: rss, url: "https://newsletter.pragmaticengineer.com/feed", paywalled: true}
  - {name: GitHub Blog (Engineering), type: rss, url: "https://github.blog/engineering/feed/"}
  - {name: GitHub Blog (AI & ML), type: rss, url: "https://github.blog/ai-and-ml/feed/"}
```

Feed URLs are best-known values. Task 17's smoke test confirms them, and the owner can edit them freely.
