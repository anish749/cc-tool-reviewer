package claudecli

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The live tests below drive the real `claude` binary end-to-end. They make an
// actual model call using the machine's Claude login, so they are opt-in:
//
//	CLAUDE_LIVE_TEST=1 go test ./internal/claudecli/ -run Live -v
//
// They are skipped when CLAUDE_LIVE_TEST is unset or `claude` is not on PATH.
func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("CLAUDE_LIVE_TEST") == "" {
		t.Skip("set CLAUDE_LIVE_TEST=1 to run live claude CLI integration tests")
	}
	if _, err := exec.LookPath(binaryName); err != nil {
		t.Skipf("%q not found on PATH", binaryName)
	}
}

func TestLive_Text(t *testing.T) {
	requireLive(t)

	c := New(WithTimeout(60 * time.Second))
	sys := "You are a terse echo. Reply with only the single word the user asks for, no punctuation."

	got, err := c.Text(context.Background(), sys, "Respond with the single word: pong")
	if err != nil {
		t.Fatalf("live Text: %v", err)
	}
	t.Logf("live Text reply: %q", got)
	if got == "" {
		t.Fatal("live Text returned empty output")
	}
	if !strings.Contains(strings.ToLower(got), "pong") {
		t.Fatalf("expected reply to contain 'pong', got %q", got)
	}
}

func TestLive_JSON_ReviewShape(t *testing.T) {
	requireLive(t)

	c := New(WithTimeout(60 * time.Second))
	sys := `You review shell commands for a CLI tool. Respond with ONLY a JSON object, no prose and no code fences:
{"decision": "allow" or "ask", "reason": "brief reason"}
Reply "allow" for obviously read-only commands.`
	user := "Tool: Bash\nInput: {\"command\": \"ls -la\"}"

	var out struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := c.JSON(context.Background(), sys, user, &out); err != nil {
		t.Fatalf("live JSON: %v", err)
	}
	t.Logf("live JSON decision=%q reason=%q", out.Decision, out.Reason)

	switch strings.ToLower(strings.TrimSpace(out.Decision)) {
	case "allow", "ask":
	default:
		t.Fatalf("unexpected decision %q", out.Decision)
	}
	if out.Reason == "" {
		t.Fatal("expected a non-empty reason")
	}
}
