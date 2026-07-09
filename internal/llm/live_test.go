package llm

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/anish/cc-tool-reviewer/internal/claudecli"
)

// TestLive_CLIClient drives the CLI-backed Client end-to-end against the real
// `claude` binary. Opt-in: it makes an actual model call using the machine's
// Claude login.
//
//	CLAUDE_LIVE_TEST=1 go test ./internal/llm/ -run Live -v
func TestLive_CLIClient(t *testing.T) {
	if os.Getenv("CLAUDE_LIVE_TEST") == "" {
		t.Skip("set CLAUDE_LIVE_TEST=1 to run live claude CLI tests")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found on PATH")
	}

	c := NewCLIClient(claudecli.WithTimeout(60 * time.Second))

	t.Run("Text", func(t *testing.T) {
		got, err := c.Text(context.Background(),
			"You are a terse echo. Reply with only the single word the user asks for, no punctuation.",
			"Respond with the single word: pong")
		if err != nil {
			t.Fatalf("Text: %v", err)
		}
		t.Logf("Text reply: %q", got)
		if !strings.Contains(strings.ToLower(got), "pong") {
			t.Fatalf("expected 'pong', got %q", got)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		sys := `You review shell commands. Respond with ONLY a JSON object, no prose and no code fences:
{"decision": "allow" or "ask", "reason": "brief reason"}
Reply "allow" for obviously read-only commands.`
		var out struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := c.JSON(context.Background(), sys, "Tool: Bash\nInput: {\"command\": \"ls -la\"}", &out); err != nil {
			t.Fatalf("JSON: %v", err)
		}
		t.Logf("JSON decision=%q reason=%q", out.Decision, out.Reason)
		switch strings.ToLower(strings.TrimSpace(out.Decision)) {
		case "allow", "ask":
		default:
			t.Fatalf("unexpected decision %q", out.Decision)
		}
	})
}
