// Package claudecli wraps the `claude` command-line binary for one-shot,
// non-interactive completions.
//
// It runs `claude -p` (print mode) with the default Claude Code system prompt
// replaced by the caller's, so the request carries only the caller's tokens and
// authenticates with the machine's existing Claude login instead of an
// ANTHROPIC_API_KEY. Complete returns the model's raw reply; higher layers
// decide how to interpret it (see internal/llm).
package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultModel matches the Anthropic SDK reviewer's model (Haiku 4.5).
	DefaultModel = "claude-haiku-4-5"
	// DefaultTimeout bounds a single CLI call. CLI cold-start is slower than a
	// direct API call, so this is more generous than the SDK path's timeout.
	DefaultTimeout = 30 * time.Second
	// binaryName is the CLI looked up on PATH by default.
	binaryName = "claude"
)

// envelope is the JSON object emitted by `claude --output-format json`.
type envelope struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// runnerFunc executes a command and returns its stdout. It is a field on Client
// so tests can substitute a fake without spawning a real process.
type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Client is a reusable, one-shot wrapper around the `claude` CLI. The zero
// value is not usable; construct one with New.
type Client struct {
	bin     string
	model   string
	timeout time.Duration
	run     runnerFunc
}

// Option configures a Client.
type Option func(*Client)

// WithModel overrides the model passed to `--model`. Empty values are ignored.
func WithModel(model string) Option {
	return func(c *Client) {
		if model != "" {
			c.model = model
		}
	}
}

// WithTimeout overrides the per-call timeout. A non-positive duration disables
// the client-side timeout, leaving the call bounded only by the caller's ctx.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithBinary overrides the CLI binary name or path (default "claude"). Empty
// values are ignored.
func WithBinary(path string) Option {
	return func(c *Client) {
		if path != "" {
			c.bin = path
		}
	}
}

// New builds a Client with sane defaults, applying opts in order.
func New(opts ...Option) *Client {
	c := &Client{
		bin:     binaryName,
		model:   DefaultModel,
		timeout: DefaultTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.run == nil {
		c.run = execRunner
	}
	return c
}

// Complete runs prompt with systemPrompt in one-shot mode and returns the
// model's raw reply. An empty systemPrompt keeps Claude Code's default system
// prompt. Trimming and JSON decoding are the caller's concern.
func (c *Client) Complete(ctx context.Context, systemPrompt, prompt string) (string, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	stdout, err := c.run(ctx, c.bin, buildArgs(c.model, systemPrompt, prompt)...)
	if err != nil {
		return "", err
	}

	var env envelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		return "", fmt.Errorf("parse CLI output: %w", err)
	}
	if env.IsError {
		return "", fmt.Errorf("claude CLI error: %s", env.Result)
	}
	return env.Result, nil
}

// buildArgs assembles the `claude` argument vector for a one-shot call. The user
// prompt is passed positionally after "--"; the system prompt, when non-empty,
// replaces the default Claude Code prompt. The isolation flags keep the call
// hermetic: no tools, no local/user/project settings, no session on disk.
func buildArgs(model, systemPrompt, prompt string) []string {
	args := []string{
		"-p",
		"--model", model,
		"--output-format", "json",
		"--no-session-persistence",
		"--tools", "", // "" disables all built-in tools
		"--setting-sources", "", // load no user/project/local settings
	}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	return append(args, "--", prompt)
}

// execRunner is the default runnerFunc: it runs the binary and returns stdout,
// wrapping any failure together with captured stderr.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}
