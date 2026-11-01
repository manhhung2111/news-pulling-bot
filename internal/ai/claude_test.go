package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaude writes an executable shell script standing in for the claude CLI.
func fakeClaude(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeReturnsResult(t *testing.T) {
	bin := fakeClaude(t, `cat >/dev/null; echo '{"type":"result","is_error":false,"result":"hi"}'`)
	got, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "prompt")
	if err != nil || got != "hi" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestClaudePassesPromptOnStdinAndFlags(t *testing.T) {
	out := filepath.Join(t.TempDir(), "stdin")
	args := filepath.Join(t.TempDir(), "args")
	bin := fakeClaude(t, `cat > `+out+`; echo "$@" > `+args+`; echo '{"is_error":false,"result":"ok"}'`)
	if _, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "the prompt"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if string(b) != "the prompt" {
		t.Errorf("stdin = %q", b)
	}
	a, _ := os.ReadFile(args)
	if strings.TrimSpace(string(a)) != "-p --output-format json" {
		t.Errorf("args = %q", a)
	}
}

func TestClaudeIsErrorFails(t *testing.T) {
	bin := fakeClaude(t, `cat >/dev/null; echo '{"is_error":true,"result":"Usage limit reached"}'`)
	_, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "Usage limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeNonZeroExitFails(t *testing.T) {
	bin := fakeClaude(t, `cat >/dev/null; echo "not logged in" >&2; exit 1`)
	_, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeEmptyResultFails(t *testing.T) {
	bin := fakeClaude(t, `cat >/dev/null; echo '{"is_error":false,"result":""}'`)
	if _, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClaudeTimeout(t *testing.T) {
	bin := fakeClaude(t, `exec sleep 5`)
	start := time.Now()
	_, err := NewClaude(bin, 100*time.Millisecond, "").Complete(context.Background(), "p")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("err = %v after %v", err, time.Since(start))
	}
}

func TestClaudePassesOAuthEnv(t *testing.T) {
	bin := fakeClaude(t, `cat >/dev/null; printf '{"is_error":false,"result":"%s"}' "$CLAUDE_CODE_OAUTH_TOKEN"`)
	got, err := NewClaude(bin, time.Minute, "tok123").Complete(context.Background(), "p")
	if err != nil || got != "tok123" {
		t.Fatalf("got %q, %v", got, err)
	}
}
