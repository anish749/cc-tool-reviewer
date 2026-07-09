package main

import (
	"strings"
	"testing"
)

func TestNewReviewer_SelectsAPIByDefault(t *testing.T) {
	t.Setenv(EnvUseCLIClient, "")
	r := NewReviewer([]string{"Bash(ls:*)"})
	if _, ok := r.(*apiReviewer); !ok {
		t.Fatalf("expected *apiReviewer when %s is unset, got %T", EnvUseCLIClient, r)
	}
}

func TestNewReviewer_SelectsCLIWhenEnvSet(t *testing.T) {
	t.Setenv(EnvUseCLIClient, "1")
	r := NewReviewer([]string{"Bash(ls:*)"})
	if _, ok := r.(*cliReviewer); !ok {
		t.Fatalf("expected *cliReviewer when %s is set, got %T", EnvUseCLIClient, r)
	}
}

func TestParseReviewResponse(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantDec    string
		wantReason string // "" = don't assert
	}{
		{"allow", `{"decision":"allow","reason":"ok"}`, "allow", "ok"},
		{"ask", `{"decision":"ask","reason":"nope"}`, "ask", "nope"},
		{"uppercase normalized", `{"decision":"ALLOW","reason":"ok"}`, "allow", "ok"},
		{"unknown decision coerced to ask", `{"decision":"maybe","reason":"?"}`, "ask", "?"},
		{"deny coerced to ask", `{"decision":"deny","reason":"x"}`, "ask", "x"},
		{"fenced json", "```json\n{\"decision\":\"allow\",\"reason\":\"ok\"}\n```", "allow", "ok"},
		{"malformed coerced to ask", "not json at all", "ask", "could not parse reviewer response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseReviewResponse(tc.in)
			if got.Decision != tc.wantDec {
				t.Errorf("decision = %q, want %q", got.Decision, tc.wantDec)
			}
			if tc.wantReason != "" && got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
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
