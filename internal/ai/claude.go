package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// claudeCLI runs Claude Code headless (`claude -p`) on the owner's subscription.
type claudeCLI struct {
	bin     string
	timeout time.Duration
	token   string
}

func NewClaude(bin string, timeout time.Duration, oauthToken string) Provider {
	return &claudeCLI{bin: bin, timeout: timeout, token: oauthToken}
}

func (c *claudeCLI) Name() string { return "claude" }

func (c *claudeCLI) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, "-p", "--output-format", "json")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = os.Environ()
	if c.token != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+c.token)
	}
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("claude timed out after %v", c.timeout)
		}
		return "", fmt.Errorf("claude failed: %w: %s", err, head(stderr.String()+stdout.String(), 300))
	}
	var resp struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("claude output not JSON: %s", head(stdout.String(), 300))
	}
	if resp.IsError {
		return "", fmt.Errorf("claude error: %s", head(resp.Result, 300))
	}
	if strings.TrimSpace(resp.Result) == "" {
		return "", errors.New("claude returned an empty result")
	}
	return resp.Result, nil
}

func head(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
