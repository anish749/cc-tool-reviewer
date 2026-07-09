package llm

import (
	"context"
	"errors"
	"testing"
)

// stubClient builds a Client whose raw reply and error are fixed, so the shared
// Text/JSON handling can be tested without any backend.
func stubClient(reply string, err error) *client {
	return &client{
		generate: func(_ context.Context, _, _ string) (string, error) {
			return reply, err
		},
	}
}

func TestText_Trims(t *testing.T) {
	got, err := stubClient("  hello  \n", nil).Text(context.Background(), "sys", "p")
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if got != "hello" {
		t.Fatalf("Text = %q, want %q", got, "hello")
	}
}

func TestText_PropagatesError(t *testing.T) {
	sentinel := errors.New("transport down")
	_, err := stubClient("", sentinel).Text(context.Background(), "sys", "p")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Text error = %v, want %v", err, sentinel)
	}
}

func TestJSON_Unmarshal(t *testing.T) {
	var out struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	err := stubClient(`{"decision":"allow","reason":"ok"}`, nil).
		JSON(context.Background(), "sys", "p", &out)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if out.Decision != "allow" || out.Reason != "ok" {
		t.Fatalf("decoded = %+v", out)
	}
}

func TestJSON_StripsFences(t *testing.T) {
	var out map[string]string
	reply := "```json\n{\"k\":\"v\"}\n```"
	if err := stubClient(reply, nil).JSON(context.Background(), "sys", "p", &out); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if out["k"] != "v" {
		t.Fatalf("decoded = %v", out)
	}
}

func TestJSON_ParseError(t *testing.T) {
	var out map[string]any
	err := stubClient("not json at all", nil).JSON(context.Background(), "sys", "p", &out)

	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if perr.Raw != "not json at all" {
		t.Fatalf("ParseError.Raw = %q, want the raw reply", perr.Raw)
	}
}

func TestJSON_PropagatesTransportError(t *testing.T) {
	// A backend failure must surface as a plain error, NOT a ParseError, so the
	// caller can tell a broken call apart from a malformed reply.
	sentinel := errors.New("api 500")
	var out map[string]any
	err := stubClient("", sentinel).JSON(context.Background(), "sys", "p", &out)

	var perr *ParseError
	if errors.As(err, &perr) {
		t.Fatalf("transport error misclassified as ParseError: %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("JSON error = %v, want %v", err, sentinel)
	}
}

func TestStripCodeFences(t *testing.T) {
	cases := map[string]string{
		"plain":            "plain",
		"```json\n{}\n```": "{}",
		"```\n{}\n```":     "{}",
		"  ```json {} ```": "{}",
	}
	for in, want := range cases {
		if got := stripCodeFences(in); got != want {
			t.Errorf("stripCodeFences(%q) = %q, want %q", in, got, want)
		}
	}
}
