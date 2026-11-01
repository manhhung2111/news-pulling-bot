# AI Engineering Digest — Design

Date: 2026-11-01
Status: Draft for owner review

## 1. Intent

A daily Discord briefing for one software engineer (the owner). It surfaces the few things published recently on **applying AI coding agents across the whole software development lifecycle (SDLC)**. Two strands are in scope:

1. **Making the agent itself effective:** setting up its environment and giving it tools, context, knowledge, and memory of past decisions. This was the original focus.
2. **Using agents at each stage of the SDLC:** requirements and specs, design and architecture, implementation, code review, testing, CI/CD and release, incident response and debugging, documentation, migrations and maintenance, and team practices around agents.

Examples of what the owner reads and acts on: a post on packaging a workflow into a Claude skill, and a post on building a context hub for coding agents. In the same spirit, a post on wiring an agent into PR review or test generation in CI would also qualify.

Out of scope: general software engineering content with no AI or agent angle.

**Why:** good material on this is spread across noisy places (Hacker News, Reddit) and a few quiet, high-signal ones (the Anthropic blog). The owner wants a short daily list they trust, without having to dig for it.

**The failure that matters:** bad picks. If links are weak, shallow, or off-topic, the owner will mute the channel. Quality of picks and summaries matters more than everything else, including how many items get sent.

## 2. Goals

- One post every day at **08:00 UTC+7** (Asia/Ho_Chi_Minh), which is 01:00 UTC, including quiet days.
- **Up to 3–5 items** per day. This is a ceiling, not a quota.
- Each item gets a breakdown with:
  - **Summary** with citations back to the source
  - **Try this:** concrete, actionable steps
  - **Why it works:** the mechanism behind the approach
  - **Problem it solves**
  - **Community TL;DR** for r/ClaudeAI items, when the subreddit's mod bot has posted one
- **No link is ever sent twice** across days.
- **Sources are configurable** in a config file. Adding or removing a blog, subreddit, or feed needs no code change.
- **Learns the owner's taste** from their 👍/👎 reactions and their replies in the daily threads.

## 3. Non-goals

- General AI news: funding, model launches, benchmark races, industry gossip. These are in scope only when they come with concrete practices for coding agents.
- Multi-user or multi-server support. This is one owner, one channel.
- A web UI or dashboard.
- Real-time or more-than-daily alerts.

## 4. Anti-goals (failure even if it "works")

- Padding a thin day with weak items to reach 3–5.
- Picking by popularity: a 900-point Hacker News thread of hot takes loses to a 40-point post with a concrete technique.
- Summaries that just restate the title, or "Try this" steps too vague to act on ("consider using AI more").
- Claims in a summary that aren't supported by the source.
- Silent failure, where the morning post just doesn't show up.

## 5. Owner decisions (fixed)

| Area | Decision |
|---|---|
| Language | **Go**, built as a single binary |
| Host | Owner's **EC2** instance (2 vCPU, 2 GB RAM) |
| Schedule | Daily at 08:00 Asia/Ho_Chi_Minh |
| Starting sources | Anthropic blog, Hacker News, r/ClaudeAI, Simon Willison's blog, Martin Fowler's site, The Pragmatic Engineer, GitHub blog (engineering / AI) |
| Scope | AI coding agents: making the agent itself effective, plus applying agents across the SDLC. General SE content with no AI angle is out. |
| Reddit access | **No Reddit API key.** Use public RSS feeds. |
| Reddit TL;DR | Pull the r/ClaudeAI mod bot's discussion-summary comment and show it as the Community TL;DR |
| Primary AI | **Claude Code CLI** in headless mode (`claude -p`) on the owner's **Claude Pro** subscription. No Anthropic API key. |
| Fallback AI | **Google Gemini** on the owner's free API key. It often hits rate limits at peak times, so it is the fallback only. |
| Discord delivery | **A Discord bot** the owner will create, not a webhook |
| Message layout | **Option B:** a short headline list in the channel, plus a thread holding one detailed message per item |
| Quiet days | Always post. Send fewer items, or say plainly that nothing met the bar. |
| Feedback | The owner reacts 👍/👎 and replies in the threads. The bot uses this to tune future picks. |

## 6. How it works

### 6.1 Run shape

A single run-once binary, triggered daily by cron or a systemd timer on EC2. Each run:

1. **Collect** candidates from every configured source, published within the lookback window (default 48h).
2. **Drop** anything already sent, and anything already shown to the AI on an earlier run (see 6.3).
3. **Read feedback** from the last 14 days of digest threads (see 6.6).
4. **Select and write** with Claude Code. If that fails, use Gemini (see 6.4).
5. **Post** to Discord (see 6.5).
6. **Record** what was sent.

### 6.2 Sources (configurable)

Sources are defined in a config file (YAML, for example). Each entry has a `type`, an `enabled` flag, and type-specific settings. Initial types:

- **`anthropic_blog`** (or a generic `html_listing` type). Watch anthropic.com's News and Engineering listing pages for new posts. Low volume; nearly everything is a candidate.
- **`hackernews`**. Recent stories from the official HN API or the Algolia HN search API. Filter loosely by AI keywords such as Claude, agent, agentic, LLM, AI, MCP, Cursor, Copilot, Codex, context, prompt, skill, and RAG, plus a minimum score. The SDLC angle (review, testing, CI, incidents, and so on) is judged by the AI, not by keywords. The keyword list and score threshold are in config.
- **`reddit_rss`**. A subreddit's public RSS feed (initially r/ClaudeAI, configurable listing such as `top?t=day` or `new`). For shortlisted posts, also fetch the thread's comments RSS to find the mod bot's TL;DR comment. The bot username and/or marker phrase is set in config. *Note: we could not inspect this bot during design because Reddit blocked our requests. The builder should confirm its username and comment format on a live thread.*
- **`rss`**. A generic RSS/Atom feed, so the owner can add any blog or newsletter later. Starting `rss` sources (the builder confirms the current feed URLs):
  - **Simon Willison's blog** (simonwillison.net). Very frequent posts, including many short link posts. Low-effort link posts should rarely pass the bar on their own.
  - **Martin Fowler's site** (martinfowler.com). The feed covers the whole site. The "Exploring Generative AI" series is the most relevant part; the AI filters out the rest.
  - **The Pragmatic Engineer** (newsletter.pragmaticengineer.com). Many posts are paywalled, so the feed and page may show only a preview. Summarize only what's visible, never invent the rest, and mark these items "partly paywalled."
  - **GitHub blog**. Prefer the engineering and AI & ML category feeds over the full-blog feed, which is mostly product announcements.

**Reddit blocking:** Reddit often blocks requests from AWS IP addresses. If a Reddit fetch fails, the run continues without it, and the headline message says "r/ClaudeAI unavailable today." Each source failure is isolated: one source failing never stops the digest. **Owner decision:** if the block turns out to be permanent from EC2, accept losing Reddit. No proxy and no other workaround.

### 6.3 De-duplication

- A SQLite file stores every URL that was sent, normalized to strip tracking params, trailing slashes, and similar noise.
- The same article can arrive from several places. For example, an Anthropic post can also show up on HN and Reddit. These should collapse into one item. Prefer the original article as the main link, and add the HN or Reddit discussion as secondary links.
- Never resend a URL that is in the history.
- **Candidates are shown to the AI once.** The 48h lookback means a candidate could otherwise appear on two runs in a row. Every candidate offered to the AI is recorded as `seen`. A seen-but-not-picked candidate is not offered again, which keeps prompts small and stops weak HN/Reddit posts from coming back daily. Exception: if a run fails before posting, its candidates are not marked seen.

### 6.4 Selection and writing (AI)

**Primary: Claude Code headless.** The owner runs `claude setup-token` once on EC2 to get a long-lived token from their Pro subscription. The binary calls `claude -p` with a structured prompt and requests JSON output. Claude may fetch the article pages itself to read them in full.

The prompt includes:
- **The fixed quality bar.** An item must teach a concrete technique an engineer could apply, either to their coding-agent setup (tools, context, knowledge, skills or workflows, memory, decision logs) or to using agents at a stage of the SDLC (specs, design, review, testing, CI/CD, incidents, docs, migrations, team process). Reject hype, opinion-only pieces, launch announcements without practices, and beginner content the owner has signaled they don't need.
- **The candidate list**, with titles, URLs, sources, scores, short excerpts, and the Reddit TL;DR where present.
- **The owner's taste notes** from feedback (6.6).
- **The output contract.** A JSON list of 0–5 items, each with title, primary URL, discussion URLs, source label, one-line hook (for the headline list), summary, try-this steps, why it works, problem it solves, community TL;DR (optional), and citations.

**Citations:** each statement in the summary, Why it works, and Problem it solves must point to a source URL: the article, or the HN or Reddit discussion. Where practical, use a short quoted phrase or a text-fragment link. Claims not supported by a source must not appear.

**Fallback: Gemini.** If Claude Code errors out, times out, returns invalid JSON, or hits its usage limit, run the same prompt and contract against Gemini on the free key, retrying with backoff. The model name goes in config. A digest made by the fallback says so in a small footer.

**Validation:** check the JSON against the contract before posting. URLs must come from the candidate list; drop any item whose URL isn't there.

### 6.5 Discord delivery

The owner creates a Discord application and bot, enables **Message Content Intent**, and invites the bot to their server with permission to send messages, create public threads, send messages in threads, read message history, and add reactions.

Each day:
1. **Headline message** in the configured channel:
   `☀️ <Weekday, Mon D> — N picks`, then a numbered list of `title — source` with a one-line hook each. Any notes come last: unavailable sources, or "made with fallback model."
2. **Thread** started from that message, named for the date.
3. **One message per item** in the thread, as an embed or formatted text. Each has a title link, the source, the summary with numbered citations, Try this, Why it works, Problem it solves, an optional Community TL;DR, and discussion links. Respect Discord limits: 2000 characters of content per message, and 6000 characters total across a message's embeds. Split or trim if needed.
4. The bot seeds 👍 and 👎 reactions on each item message so the owner can tap one.

**Quiet day:** the headline message says "Nothing met the bar today" and lists 0 items, or lists 1–2 items when only that many qualified. Don't create a thread when there are zero items.

**Run failure:** if the run fails before posting (for example, every AI path fails), the bot posts a single short error line in the channel. Full details go to the local log.

### 6.6 Learning from feedback

- Each run reads the digest threads from the last 14 days.
- Only feedback from the **owner's Discord user ID** (in config) counts.
- Signals: 👍 and 👎 on item messages, and the owner's free-text replies in the thread (for example: "too basic", "more like this for Go", "this source is always hype").
- Feedback is stored in SQLite, linked to the item it was about.
- Feedback goes into the prompt as **taste notes**:
  - Recent feedback is included in full, with the item title, the source, and the owner's reaction or comment.
  - Older feedback is condensed into a short running summary of the owner's preferences, which Claude updates periodically. This keeps the prompt small.
- **The core quality bar (6.4) never changes.** Feedback only steers choices within it.

## 7. Configuration

One config file plus environment variables for secrets.

- **Config:** sources (6.2), lookback window, max items, HN keywords and min score, the Reddit bot identifier, the Gemini model name, the Discord channel ID, the owner's Discord user ID, the feedback window, and timezone and schedule notes.
- **Secrets (env vars):** `DISCORD_BOT_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN` (from `claude setup-token`), `GEMINI_API_KEY`.

## 8. Operations

- Runs from cron (`CRON_TZ=Asia/Ho_Chi_Minh`, `0 8 * * *`) or an equivalent systemd timer.
- Logs to a file or journald, with one summary line per run: candidates per source, items picked, which AI path was used, and errors.
- A `--dry-run` flag prints the digest to stdout instead of posting, for tuning the prompt.
- Uses a lock so overlapping runs can't double-post.

## 9. Left to the builder

- Go project layout, libraries (such as gofeed, go-readability, discordgo, and Google's genai SDK), and the config file format.
- Exact scraping for the Anthropic pages.
- Exact prompt wording, as long as it enforces the quality bar, citations, and the output contract.
- The SQLite schema.
- Retry and backoff details.
- How to install and update the binary on EC2.

## 10. Calls made during design (not explicitly confirmed by the owner)

- Lookback window of 48h.
- SQLite for history and feedback.
- Feedback window of 14 days, with older feedback condensed into a running summary.
- Only the owner's own reactions and replies count as feedback.
- When Claude fails, the fallback is a full re-run on Gemini, not a partial one.
- The headline message notes which sources were unavailable and whether the fallback model was used.
- No thread is created on a zero-item day.
- The bot seeds 👍/👎 reactions on each item.
- "Links configurable" is read as: **sources are defined in config**.
- Each candidate is shown to the AI only once. Passed-over candidates are not re-offered the next day.
- Out-of-scope boundary: general software engineering content with no AI or agent angle is excluded.
- Paywalled Pragmatic Engineer items are summarized from the visible preview only and labeled "partly paywalled."
- For the GitHub blog, use the engineering and AI category feeds rather than the full-blog feed.
