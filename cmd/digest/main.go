// Command digest builds and posts the daily AI engineering digest.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/mhung2111/ai-eng-digest/internal/ai"
	"github.com/mhung2111/ai-eng-digest/internal/config"
	"github.com/mhung2111/ai-eng-digest/internal/discord"
	"github.com/mhung2111/ai-eng-digest/internal/enrich"
	"github.com/mhung2111/ai-eng-digest/internal/pipeline"
	"github.com/mhung2111/ai-eng-digest/internal/sources"
	"github.com/mhung2111/ai-eng-digest/internal/store"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "print the digest instead of posting it")
	flag.Parse()
	os.Exit(run(*cfgPath, *dryRun))
}

func run(cfgPath string, dryRun bool) int {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("load config", "err", err)
		return 2
	}

	var api discord.API
	if !dryRun {
		if cfg.Secrets.DiscordToken == "" {
			log.Error("DISCORD_BOT_TOKEN is not set (use --dry-run to test without Discord)")
			return 2
		}
		if api, err = discord.NewAPI(cfg.Secrets.DiscordToken); err != nil {
			log.Error("discord client", "err", err)
			return 2
		}
	}

	release, err := acquireLock(cfg.DBPath + ".lock")
	if err != nil {
		log.Error("lock", "err", err)
		return 1
	}
	defer release()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("open store", "err", err)
		return 1
	}
	defer st.Close()

	hc := &http.Client{Timeout: 30 * time.Second}
	deps := sources.Deps{HTTP: hc, UserAgent: cfg.UserAgent}
	srcs, err := sources.Build(cfg.Sources, deps)
	if err != nil {
		log.Error("build sources", "err", err)
		return 2
	}
	var tldr pipeline.TLDRFetcher
	for _, sc := range cfg.Sources {
		if sc.Type == "reddit_rss" && sc.IsEnabled() {
			tldr = sources.NewReddit(sc, deps, "https://www.reddit.com")
			break
		}
	}

	providers := []ai.Provider{ai.NewClaude(cfg.AI.ClaudeBin, cfg.AI.ClaudeTimeout, cfg.Secrets.ClaudeOAuthToken)}
	if g, err := ai.NewGemini(cfg.Secrets.GeminiAPIKey, cfg.AI.GeminiModel); err == nil {
		providers = append(providers, g)
	} else {
		log.Info("gemini fallback disabled", "reason", err)
	}

	sum, err := pipeline.Run(context.Background(), pipeline.Deps{
		Cfg: cfg, Store: st, Sources: srcs, TLDR: tldr,
		AI:      ai.Chain{Providers: providers},
		Discord: api,
		Article: func(ctx context.Context, url string) (string, error) {
			return enrich.ArticleText(ctx, hc, cfg.UserAgent, url)
		},
		Now: time.Now, Out: os.Stdout, Log: log,
	}, dryRun)

	errText := ""
	if err != nil {
		errText = err.Error()
	}
	log.Info("digest run", "dry_run", dryRun, "candidates_per_source", fmt.Sprint(sum.PerSource),
		"offered", sum.Offered, "picked", sum.Picked, "provider", sum.Provider, "error", errText)
	if err != nil {
		return 1
	}
	return 0
}
