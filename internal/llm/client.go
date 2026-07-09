// Package llm provides a small one-shot LLM client used by the reviewer.
//
// A Client turns a single (system prompt, user prompt) pair into either plain
// text or a JSON-decoded value. The two concerns are split cleanly: producing
// the model's raw reply is backend-specific (Anthropic SDK or the local Claude
// CLI), while trimming text and decoding JSON is shared across every backend
// and implemented exactly once here.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Client is a one-shot LLM caller. Text returns the model's reply as plain
// text; JSON decodes that reply into out, tolerating ```json ... ``` fences.
type Client interface {
	Text(ctx context.Context, systemPrompt, prompt string) (string, error)
	JSON(ctx context.Context, systemPrompt, prompt string, out any) error
}

// generateFunc produces the model's raw reply for a system+user prompt. It is
// the single operation each backend supplies; Text and JSON are layered on top.
type generateFunc func(ctx context.Context, systemPrompt, prompt string) (string, error)

// client is the one Client implementation. Backends differ only in generate.
type client struct {
	generate generateFunc
}

func (c *client) Text(ctx context.Context, systemPrompt, prompt string) (string, error) {
	raw, err := c.generate(ctx, systemPrompt, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(raw), nil
}

func (c *client) JSON(ctx context.Context, systemPrompt, prompt string, out any) error {
	text, err := c.Text(ctx, systemPrompt, prompt)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(stripCodeFences(text)), out); err != nil {
		return &ParseError{Raw: text, Err: err}
	}
	return nil
}

// ParseError reports that the model's reply was not valid JSON. It carries the
// raw text so callers can tell a malformed reply apart from a transport failure
// (a plain error) and react differently.
type ParseError struct {
	Raw string
	Err error
}

func (e *ParseError) Error() string { return fmt.Sprintf("llm: parse JSON response: %v", e.Err) }
func (e *ParseError) Unwrap() error { return e.Err }

// stripCodeFences removes a leading ```json / ``` fence and a trailing ``` that
// a model sometimes wraps around its JSON reply.
func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}
