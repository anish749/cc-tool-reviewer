package claudecli

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLive_Complete drives the real `claude` binary end-to-end. It makes an
// actual model call using the machine's Claude login. Skipped in short mode:
//
//	go test ./internal/llm/claudecli/ -run Live -v
func TestLive_Complete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	if _, err := exec.LookPath(binaryName); err != nil {
		t.Skipf("%q not found on PATH", binaryName)
	}

	c := New("claude-haiku-4-5")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sys := "You are a terse echo. Reply with only the single word the user asks for, no punctuation."

	got, err := c.Complete(ctx, sys, "Respond with the single word: pong")
	if err != nil {
		t.Fatalf("live Complete: %v", err)
	}
	t.Logf("live Complete reply: %q", got)
	if !strings.Contains(strings.ToLower(got), "pong") {
		t.Fatalf("expected reply to contain 'pong', got %q", got)
	}
}
