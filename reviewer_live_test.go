package main

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

// TestLive_CLIReviewer drives the reviewer end-to-end over the CLI-backed LLM
// client against the real `claude` binary — the path taken when
// USE_CLAUDE_CLI_CLIENT is set. Skipped in short mode:
//
//	go test . -run TestLive_CLIReviewer -v
func TestLive_CLIReviewer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
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
