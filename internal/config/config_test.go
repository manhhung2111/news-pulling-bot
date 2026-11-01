package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalYAML = `
db_path: ./t.db
discord:
  channel_id: "123"
  owner_user_id: "456"
sources:
  - name: Blog
    type: rss
    url: https://example.com/feed
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAppliesDefaults(t *testing.T) {
	c, err := Load(writeConfig(t, minimalYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.LookbackHours != 48 || c.MaxItems != 5 || c.ShortlistSize != 8 || c.FeedbackDays != 14 {
		t.Errorf("numeric defaults wrong: %+v", c)
	}
	if c.Timezone != "Asia/Ho_Chi_Minh" {
		t.Errorf("timezone = %q", c.Timezone)
	}
	if c.AI.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("gemini model = %q", c.AI.GeminiModel)
	}
	if c.AI.ClaudeTimeout != 10*time.Minute {
		t.Errorf("claude timeout = %v", c.AI.ClaudeTimeout)
	}
	if c.AI.ClaudeBin != "claude" {
		t.Errorf("claude bin = %q", c.AI.ClaudeBin)
	}
	if c.UserAgent != "ai-eng-digest/0.1" {
		t.Errorf("user agent = %q", c.UserAgent)
	}
	if c.Location().String() != "Asia/Ho_Chi_Minh" {
		t.Errorf("location = %v", c.Location())
	}
}

func TestLoadReadsSecretsFromEnv(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "d")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "c")
	t.Setenv("GEMINI_API_KEY", "g")
	c, err := Load(writeConfig(t, minimalYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.Secrets != (Secrets{DiscordToken: "d", ClaudeOAuthToken: "c", GeminiAPIKey: "g"}) {
		t.Errorf("secrets = %+v", c.Secrets)
	}
}

func TestLoadRejectsMissingRequired(t *testing.T) {
	cases := map[string]string{
		"channel_id":    strings.Replace(minimalYAML, `channel_id: "123"`, "", 1),
		"owner_user_id": strings.Replace(minimalYAML, `owner_user_id: "456"`, "", 1),
		"db_path":       strings.Replace(minimalYAML, "db_path: ./t.db", "", 1),
	}
	for field, body := range cases {
		_, err := Load(writeConfig(t, body))
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("missing %s: err = %v", field, err)
		}
	}
}

func TestLoadRejectsUnknownSourceType(t *testing.T) {
	_, err := Load(writeConfig(t, strings.Replace(minimalYAML, "type: rss", "type: foo", 1)))
	if err == nil {
		t.Fatal("expected error for unknown source type")
	}
}

func TestLoadRejectsMaxItemsOver5(t *testing.T) {
	_, err := Load(writeConfig(t, minimalYAML+"max_items: 6\n"))
	if err == nil {
		t.Fatal("expected error for max_items 6")
	}
}
