package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

func TestReview_Allow(t *testing.T) {
	f := &llm.Fake{Reply: `{"decision":"allow","reason":"read-only"}`}
	r := NewReviewer(f, []string{"Bash(ls:*)"})

	got, err := r.Review("Bash", json.RawMessage(`{"command":"ls -la"}`))
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Decision != "allow" || got.Reason != "read-only" {
		t.Fatalf("decision = %+v", got)
	}
	// The reviewer must pass its system prompt and a fenced tool-call user message.
	if !strings.Contains(f.GotSystem, "Bash(ls:*)") {
		t.Errorf("system prompt missing allow rule: %q", f.GotSystem)
	}
	if !strings.Contains(f.GotPrompt, "<tool_call>\nTool: Bash\nInput: {\"command\":\"ls -la\"}\n</tool_call>") {
		t.Errorf("user message = %q", f.GotPrompt)
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
		r := NewReviewer(&llm.Fake{Reply: raw}, nil)
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
	f := &llm.Fake{Err: &llm.ParseError{Raw: "garbage", Err: errors.New("bad json")}}
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
	f := &llm.Fake{Err: errors.New("api unreachable")}
	r := NewReviewer(f, nil)

	_, err := r.Review("Bash", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for transport failure")
	}
	if !strings.Contains(err.Error(), "api unreachable") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewReviewLLM_BuildsClient(t *testing.T) {
	// Both branches must construct a usable client without panicking.
	t.Setenv("ANTHROPIC_API_KEY", "")
	if NewReviewLLM() == nil {
		t.Error("nil client for CLI (default) branch")
	}
	t.Setenv("ANTHROPIC_API_KEY", "x")
	if NewReviewLLM() == nil {
		t.Error("nil client for Anthropic branch")
	}
}

func TestBuildSystemPrompt_IncludesRules(t *testing.T) {
	sp := buildSystemPrompt([]string{"Bash(ls:*)", "Read(*)"})
	for _, want := range []string{"Bash(ls:*)", "Read(*)", `"allow"`, `"ask"`, "<tool_call>"} {
		if !strings.Contains(sp, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestBuildUserMessage(t *testing.T) {
	got := buildUserMessage("Bash", []byte(`{"command":"ls"}`))
	want := "Review the tool call below and reply with only your JSON verdict.\n\n<tool_call>\nTool: Bash\nInput: {\"command\":\"ls\"}\n</tool_call>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
