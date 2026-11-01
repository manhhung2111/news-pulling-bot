// Package config loads the digest's YAML config and secrets from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Timezone      string         `yaml:"timezone"`
	LookbackHours int            `yaml:"lookback_hours"`
	MaxItems      int            `yaml:"max_items"`
	ShortlistSize int            `yaml:"shortlist_size"`
	FeedbackDays  int            `yaml:"feedback_days"`
	DBPath        string         `yaml:"db_path"`
	UserAgent     string         `yaml:"user_agent"`
	Discord       DiscordConfig  `yaml:"discord"`
	AI            AIConfig       `yaml:"ai"`
	Sources       []SourceConfig `yaml:"sources"`
	Secrets       Secrets        `yaml:"-"`

	loc *time.Location
}

type DiscordConfig struct {
	ChannelID   string `yaml:"channel_id"`
	OwnerUserID string `yaml:"owner_user_id"`
}

type AIConfig struct {
	ClaudeBin     string        `yaml:"claude_bin"`
	ClaudeTimeout time.Duration `yaml:"claude_timeout"`
	GeminiModel   string        `yaml:"gemini_model"`
}

type SourceConfig struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	URL     string `yaml:"url"`
	Enabled *bool  `yaml:"enabled"` // nil means enabled

	LinkPrefix string `yaml:"link_prefix"` // html_listing

	Keywords  []string `yaml:"keywords"` // hackernews
	MinPoints int      `yaml:"min_points"`

	Subreddit  string `yaml:"subreddit"` // reddit_rss
	Listing    string `yaml:"listing"`
	Period     string `yaml:"period"`
	TLDRAuthor string `yaml:"tldr_author"`
	TLDRMarker string `yaml:"tldr_marker"`

	Paywalled bool `yaml:"paywalled"` // rss
}

// IsEnabled reports whether the source should run.
func (s SourceConfig) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

type Secrets struct {
	DiscordToken     string
	ClaudeOAuthToken string
	GeminiAPIKey     string
}

var knownSourceTypes = map[string]bool{
	"rss": true, "hackernews": true, "reddit_rss": true, "html_listing": true,
}

// Load reads the YAML file at path, applies defaults, reads secrets from env and validates.
// Missing secrets are not an error: a dry run needs none of them.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	c.Secrets = Secrets{
		DiscordToken:     os.Getenv("DISCORD_BOT_TOKEN"),
		ClaudeOAuthToken: os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Timezone == "" {
		c.Timezone = "Asia/Ho_Chi_Minh"
	}
	if c.LookbackHours == 0 {
		c.LookbackHours = 48
	}
	if c.MaxItems == 0 {
		c.MaxItems = 5
	}
	if c.ShortlistSize == 0 {
		c.ShortlistSize = 8
	}
	if c.FeedbackDays == 0 {
		c.FeedbackDays = 14
	}
	if c.UserAgent == "" {
		c.UserAgent = "ai-eng-digest/0.1"
	}
	if c.AI.ClaudeBin == "" {
		c.AI.ClaudeBin = "claude"
	}
	if c.AI.ClaudeTimeout == 0 {
		c.AI.ClaudeTimeout = 10 * time.Minute
	}
	if c.AI.GeminiModel == "" {
		c.AI.GeminiModel = "gemini-2.5-flash"
	}
}

func (c *Config) validate() error {
	var errs []error
	if c.DBPath == "" {
		errs = append(errs, errors.New("db_path is required"))
	}
	if c.Discord.ChannelID == "" {
		errs = append(errs, errors.New("discord.channel_id is required"))
	}
	if c.Discord.OwnerUserID == "" {
		errs = append(errs, errors.New("discord.owner_user_id is required"))
	}
	if c.MaxItems < 1 || c.MaxItems > 5 {
		errs = append(errs, fmt.Errorf("max_items must be 1..5, got %d", c.MaxItems))
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		errs = append(errs, fmt.Errorf("timezone: %w", err))
	}
	c.loc = loc
	for i, s := range c.Sources {
		if s.Name == "" {
			errs = append(errs, fmt.Errorf("sources[%d]: name is required", i))
		}
		if !knownSourceTypes[s.Type] {
			errs = append(errs, fmt.Errorf("sources[%d] %q: unknown type %q", i, s.Name, s.Type))
		}
	}
	return errors.Join(errs...)
}

// Location returns the configured timezone.
func (c *Config) Location() *time.Location {
	if c.loc == nil {
		return time.UTC
	}
	return c.loc
}
