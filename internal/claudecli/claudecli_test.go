package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeRunner records the command it was asked to run and returns canned output,
// so Complete can be exercised without spawning a process.
type fakeRunner struct {
	gotName string
	gotArgs []string
	stdout  []byte
	err     error
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.gotName = name
	f.gotArgs = args
	return f.stdout, f.err
}

func newTestClient(t *testing.T, r runnerFunc) *Client {
	t.Helper()
	c := New(WithBinary("claude-test"), WithModel("claude-haiku-4-5"))
	c.run = r
	return c
}

func envelopeJSON(t *testing.T, result string, isErr bool) []byte {
	t.Helper()
	b, err := json.Marshal(envelope{Type: "result", Subtype: "success", IsError: isErr, Result: result})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

func TestBuildArgs_WithSystemPrompt(t *testing.T) {
	got := buildArgs("claude-haiku-4-5", "be terse", "hello")
	want := []string{
		"-p",
		"--model", "claude-haiku-4-5",
		"--output-format", "json",
		"--no-session-persistence",
		"--tools", "",
		"--setting-sources", "",
		"--system-prompt", "be terse",
		"--", "hello",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildArgs mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestBuildArgs_NoSystemPrompt(t *testing.T) {
	got := buildArgs("m", "", "hi")
	for _, a := range got {
		if a == "--system-prompt" {
			t.Fatalf("unexpected --system-prompt in args: %#v", got)
		}
	}
	// The prompt must be the final positional arg, after the "--" separator.
	if got[len(got)-2] != "--" || got[len(got)-1] != "hi" {
		t.Fatalf("prompt not passed positionally after --: %#v", got)
	}
}

func TestComplete_Success(t *testing.T) {
	// Complete returns the envelope result verbatim (no trimming — that is the
	// caller's concern).
	r := &fakeRunner{stdout: envelopeJSON(t, "  raw reply  ", false)}
	c := newTestClient(t, r.run)

	got, err := c.Complete(context.Background(), "sys", "user prompt")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "  raw reply  " {
		t.Fatalf("Complete = %q, want %q", got, "  raw reply  ")
	}
	// The runner must be invoked with the configured binary and prompt.
	if r.gotName != "claude-test" {
		t.Fatalf("binary = %q, want claude-test", r.gotName)
	}
	if !strings.Contains(strings.Join(r.gotArgs, " "), "--system-prompt sys") {
		t.Fatalf("args missing system prompt: %v", r.gotArgs)
	}
	if r.gotArgs[len(r.gotArgs)-1] != "user prompt" {
		t.Fatalf("prompt not last arg: %v", r.gotArgs)
	}
}

func TestComplete_ErrorEnvelope(t *testing.T) {
	r := &fakeRunner{stdout: envelopeJSON(t, "model exploded", true)}
	c := newTestClient(t, r.run)

	_, err := c.Complete(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error for is_error envelope")
	}
	if !strings.Contains(err.Error(), "model exploded") {
		t.Fatalf("error missing CLI result: %v", err)
	}
}

func TestComplete_MalformedOutput(t *testing.T) {
	r := &fakeRunner{stdout: []byte("not json at all")}
	c := newTestClient(t, r.run)

	_, err := c.Complete(context.Background(), "", "user")
	if err == nil {
		t.Fatal("expected parse error for malformed CLI output")
	}
	if !strings.Contains(err.Error(), "parse CLI output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_RunnerError(t *testing.T) {
	r := &fakeRunner{err: errors.New("exit status 1: boom")}
	c := newTestClient(t, r.run)

	_, err := c.Complete(context.Background(), "", "user")
	if err == nil {
		t.Fatal("expected error from runner")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runner error not propagated: %v", err)
	}
}

func TestComplete_Timeout(t *testing.T) {
	// A runner that blocks until ctx cancellation proves the client applies its
	// own timeout to the call.
	slow := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c := New(WithBinary("claude-test"), WithTimeout(20*time.Millisecond))
	c.run = slow

	start := time.Now()
	_, err := c.Complete(context.Background(), "", "user")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestExecRunner_BadBinary(t *testing.T) {
	// The default exec runner must surface an error when the binary is missing,
	// rather than panicking or returning empty output.
	c := New(WithBinary("/nonexistent/claude-xyz"), WithTimeout(2*time.Second))
	if _, err := c.Complete(context.Background(), "", "hi"); err == nil {
		t.Fatal("expected error for missing binary")
	}
}
