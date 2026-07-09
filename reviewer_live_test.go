package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

// TestLive_CLIReviewer drives the reviewer end-to-end over the CLI-backed LLM
// client against the real `claude` binary — the path taken when
// USE_CLAUDE_CLI_CLIENT is set. Opt-in:
//
//	CLAUDE_LIVE_TEST=1 go test . -run TestLive_CLIReviewer -v
func TestLive_CLIReviewer(t *testing.T) {
	if os.Getenv("CLAUDE_LIVE_TEST") == "" {
		t.Skip("set CLAUDE_LIVE_TEST=1 to run the live CLI reviewer test")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found on PATH")
	}

	r := NewReviewer(llm.NewCLIClient(llm.WithModel("claude-haiku-4-5")), []string{"Bash(rg:*)", "Bash(go test:*)"})

	got, err := r.Review("Bash", json.RawMessage(`{"command":"ls -la"}`))
	if err != nil {
		t.Fatalf("cli reviewer Review: %v", err)
	}
	t.Logf("cli reviewer decision=%q reason=%q", got.Decision, got.Reason)
	if got.Decision != "allow" && got.Decision != "ask" {
		t.Fatalf("unexpected decision %q", got.Decision)
	}
}
