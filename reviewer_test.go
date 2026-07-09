package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

// fakeLLM is an llm.Client stub. When err is nil, JSON unmarshals raw into out,
// mimicking a real decoded reply; otherwise it returns err (a transport error
// or an *llm.ParseError, depending on what the test needs).
type fakeLLM struct {
	raw       string
	err       error
	gotSystem string
	gotPrompt string
}

func (f *fakeLLM) Text(_ context.Context, systemPrompt, prompt string) (string, error) {
	f.gotSystem, f.gotPrompt = systemPrompt, prompt
	return f.raw, f.err
}

func (f *fakeLLM) JSON(_ context.Context, systemPrompt, prompt string, out any) error {
	f.gotSystem, f.gotPrompt = systemPrompt, prompt
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.raw), out)
}

func TestReview_Allow(t *testing.T) {
	f := &fakeLLM{raw: `{"decision":"allow","reason":"read-only"}`}
	r := NewReviewer(f, []string{"Bash(ls:*)"})

	got, err := r.Review("Bash", json.RawMessage(`{"command":"ls -la"}`))
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Decision != "allow" || got.Reason != "read-only" {
		t.Fatalf("decision = %+v", got)
	}
	// The reviewer must pass its system prompt and a Tool/Input user message.
	if !strings.Contains(f.gotSystem, "Bash(ls:*)") {
		t.Errorf("system prompt missing allow rule: %q", f.gotSystem)
	}
	if f.gotPrompt != "Tool: Bash\nInput: {\"command\":\"ls -la\"}" {
		t.Errorf("user message = %q", f.gotPrompt)
	}
}

func TestReview_NormalizesDecision(t *testing.T) {
	cases := map[string]string{
		`{"decision":"ALLOW","reason":"x"}`:   "allow", // uppercase normalized
		`{"decision":" allow ","reason":"x"}`: "allow", // trimmed
		`{"decision":"deny","reason":"x"}`:    "ask",   // deny is not allow -> ask
		`{"decision":"maybe","reason":"x"}`:   "ask",   // unknown -> ask
	}
	for raw, want := range cases {
		r := NewReviewer(&fakeLLM{raw: raw}, nil)
		got, err := r.Review("Bash", json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("Review(%s): %v", raw, err)
		}
		if got.Decision != want {
			t.Errorf("Review(%s) decision = %q, want %q", raw, got.Decision, want)
		}
	}
}

func TestReview_ParseErrorIsSoftAsk(t *testing.T) {
	// A malformed model reply must become a soft "ask" (nil error) so the caller
	// escalates to the user rather than treating it as a broken reviewer.
	f := &fakeLLM{err: &llm.ParseError{Raw: "garbage", Err: errors.New("bad json")}}
	r := NewReviewer(f, nil)

	got, err := r.Review("Bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error for parse failure, got %v", err)
	}
	if got.Decision != "ask" {
		t.Fatalf("decision = %q, want ask", got.Decision)
	}
}

func TestReview_TransportErrorPropagates(t *testing.T) {
	f := &fakeLLM{err: errors.New("api unreachable")}
	r := NewReviewer(f, nil)

	_, err := r.Review("Bash", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for transport failure")
	}
	if !strings.Contains(err.Error(), "api unreachable") {
		t.Fatalf("error = %v", err)
	}
}

func TestUseCLIClient(t *testing.T) {
	t.Setenv(EnvUseCLIClient, "")
	if useCLIClient() {
		t.Error("expected false when env is empty")
	}
	t.Setenv(EnvUseCLIClient, "1")
	if !useCLIClient() {
		t.Error("expected true when env is set")
	}
}

func TestNewReviewLLM_BuildsClient(t *testing.T) {
	// Both branches must construct a usable client without panicking.
	t.Setenv(EnvUseCLIClient, "")
	if NewReviewLLM() == nil {
		t.Error("nil client for API branch")
	}
	t.Setenv(EnvUseCLIClient, "1")
	if NewReviewLLM() == nil {
		t.Error("nil client for CLI branch")
	}
}

func TestBuildSystemPrompt_IncludesRules(t *testing.T) {
	sp := buildSystemPrompt([]string{"Bash(ls:*)", "Read(*)"})
	for _, want := range []string{"Bash(ls:*)", "Read(*)", `"allow"`, `"ask"`} {
		if !strings.Contains(sp, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestBuildUserMessage(t *testing.T) {
	got := buildUserMessage("Bash", []byte(`{"command":"ls"}`))
	want := "Tool: Bash\nInput: {\"command\":\"ls\"}"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
