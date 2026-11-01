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
	dir := filepath.Join(t.TempDir(), "pwd")
	bin := fakeClaude(t, `cat > `+out+`; printf '%s|' "$@" > `+args+`; pwd > `+dir+`; echo '{"is_error":false,"result":"ok"}'`)
	if _, err := NewClaude(bin, time.Minute, "").Complete(context.Background(), "the prompt"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if string(b) != "the prompt" {
		t.Errorf("stdin = %q", b)
	}
	a, _ := os.ReadFile(args)
	if string(a) != "-p|--output-format|json|--tools||" {
		t.Errorf("args = %q (want all tools disabled)", a)
	}
	wd, _ := os.Getwd()
	pwd, _ := os.ReadFile(dir)
	if strings.TrimSpace(string(pwd)) == wd {
		t.Error("claude should run in an empty scratch directory, not the caller's working dir")
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
