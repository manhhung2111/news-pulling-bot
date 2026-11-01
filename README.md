# AI Engineering Digest

A daily Discord digest of practical posts on applying AI coding agents across the software development lifecycle. It runs once a day from cron, collects posts from the sources in `config.yaml`, has Claude Code (with Gemini as a fallback) pick and write up the best 0–5, and posts a headline list plus a thread with one breakdown per pick. React 👍/👎 or reply in the thread and later digests adapt to your taste.

Design: `docs/superpowers/specs/2026-11-01-ai-eng-digest-design.md`

## Setup on EC2

1. **Build** on your machine and copy the binary:
   ```sh
   GOOS=linux GOARCH=amd64 go build -o digest ./cmd/digest   # GOARCH=arm64 for Graviton
   scp digest ec2-user@<host>:/tmp/ && ssh ec2-user@<host> sudo mv /tmp/digest /usr/local/bin/
   ```
2. **Claude Code** on the instance: install the `claude` CLI, then run `claude setup-token` and keep the token for step 5.
3. **Discord bot**
   - In the [Developer Portal](https://discord.com/developers/applications): New Application → Bot → copy the token.
   - On the Bot page, turn on **Message Content Intent** (needed to read your replies).
   - OAuth2 → URL Generator: scope `bot`, permissions *Send Messages, Create Public Threads, Send Messages in Threads, Read Message History, Add Reactions*. Open the URL and add the bot to your server.
   - In Discord, enable Developer Mode (Settings → Advanced), then right-click the channel → Copy Channel ID, and right-click yourself → Copy User ID.
4. **Gemini key** (optional fallback): create one at Google AI Studio.
5. **Config and secrets**
   ```sh
   sudo mkdir -p /etc/ai-eng-digest /var/lib/ai-eng-digest
   sudo cp config.example.yaml /etc/ai-eng-digest/config.yaml      # set channel_id, owner_user_id, db_path: /var/lib/ai-eng-digest/digest.db
   sudo cp deploy/digest.env.example /etc/ai-eng-digest/digest.env # fill in the tokens
   sudo chmod 600 /etc/ai-eng-digest/digest.env
   ```
   Make sure the user that runs cron can write `/var/lib/ai-eng-digest` and `/var/log/ai-eng-digest.log`.
6. **Dry run first.** It prints the digest instead of posting it and doesn't touch history:
   ```sh
   . /etc/ai-eng-digest/digest.env && digest --config /etc/ai-eng-digest/config.yaml --dry-run
   ```
7. **Check Reddit from the instance.** AWS addresses are often blocked:
   ```sh
   curl -s -o /dev/null -w "%{http_code}\n" -A "ai-eng-digest/0.1" https://www.reddit.com/r/ClaudeAI/.rss
   ```
   `200` means it works. Anything else means r/ClaudeAI will show as unavailable each day, and the digest carries on without it.
8. **Reddit TL;DR bot.** Open a busy r/ClaudeAI thread, find the bot's discussion-summary comment, and put its username in `tldr_author` (or a phrase it always starts with in `tldr_marker`).
9. **Schedule:** `crontab deploy/digest.cron` (08:00 Asia/Ho_Chi_Minh daily).

## Everyday use

- **Add or remove sources** in `config.yaml`. Types: `rss`, `hackernews`, `reddit_rss`, `html_listing`. Set `enabled: false` to pause one.
- **Logs:** one `digest run` line per day with per-source counts, picks, and which model wrote it.
- **Tuning picks:** run with `--dry-run` as often as you like. Only real runs record history.

## Development

```sh
go test ./...
go run ./cmd/digest --config config.example.yaml --dry-run
```
