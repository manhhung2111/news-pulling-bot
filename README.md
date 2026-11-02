# AI Engineering Digest

A daily Discord digest of practical posts on applying AI coding agents across the software development lifecycle. It runs once a day from cron, collects posts from the sources in `config.yaml`, has Claude Code (with Gemini as a fallback) pick and write up the best 0–5, and posts a headline list plus a thread with one breakdown per pick. React 👍/👎 or reply in the thread and later digests adapt to your taste.

Design: `docs/superpowers/specs/2026-11-01-ai-eng-digest-design.md`

## Setup on EC2

1. **Build** on your machine and copy the binary plus the config templates (run from the repo root):
   ```sh
   GOOS=linux GOARCH=amd64 go build -o digest ./cmd/digest   # GOARCH=arm64 for Graviton
   scp digest config.example.yaml deploy/digest.env.example deploy/digest.cron ec2-user@<host>:/tmp/
   ssh ec2-user@<host> sudo mv /tmp/digest /usr/local/bin/
   ```
   Steps 5 and 9 run on the instance from `/tmp`, where these files landed.
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
   sudo cp /tmp/config.example.yaml /etc/ai-eng-digest/config.yaml  # set channel_id, owner_user_id, db_path: /var/lib/ai-eng-digest/digest.db
   sudo cp /tmp/digest.env.example /etc/ai-eng-digest/digest.env    # fill in the tokens
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
9. **Schedule:** `crontab /tmp/digest.cron` on the instance (08:00 Asia/Ho_Chi_Minh daily).

## Deploy from GitHub

`.github/workflows/ci.yml` runs `go vet` and the tests on every PR. On a merge to `main` it also builds `digest` for linux/arm64, uploads it to S3, and has the instance install it through AWS Systems Manager, so no SSH port or AWS key is involved. Only the binary changes. `config.yaml` and `digest.env` stay as they are on the instance.

One-time setup, in this order. Fill the placeholders (`ACCOUNT_ID`, `REGION`, `BUCKET`, `INSTANCE_ID`) in `deploy/aws/*.json` as you go, but don't commit your real values.

1. **S3 bucket** for builds: keep *Block all public access* on, and add a lifecycle rule that expires objects under `digest/` after 30 days.
2. **Instance role:** create an EC2 role with the managed policy `AmazonSSMManagedInstanceCore` plus `deploy/aws/instance-policy.json`, and attach it to the instance (Actions → Security → Modify IAM role). After a minute, `aws ssm describe-instance-information` should list the instance as `Online`.
3. **GitHub as an identity provider:** IAM → Identity providers → Add → OpenID Connect, URL `https://token.actions.githubusercontent.com`, audience `sts.amazonaws.com`.
4. **Deploy role:** create a role with `deploy/aws/github-trust-policy.json` as its trust policy and `deploy/aws/github-deploy-policy.json` as its permissions.
5. **GitHub environment:** Settings → Environments → New `production`.
   - Deployment branches: `main` only. Required reviewers: you.
   - Secret `AWS_ROLE_ARN`: the deploy role's ARN.
   - Variables `AWS_REGION`, `DEPLOY_BUCKET`, `EC2_INSTANCE_ID`.
6. **Repo settings:** Actions → General → Workflow permissions: *Read repository contents*. Then add the `test` check as required in the `main` ruleset.

## Everyday use

- **Add or remove sources** in `config.yaml`. Types: `rss`, `hackernews`, `reddit_rss`, `html_listing`. Set `enabled: false` to pause one.
- **Logs:** one `digest run` line per day with per-source counts, picks, and which model wrote it.
- **Tuning picks:** run with `--dry-run` as often as you like. Only real runs record history.

## Development

```sh
go test ./...
go run ./cmd/digest --config config.example.yaml --dry-run
```
