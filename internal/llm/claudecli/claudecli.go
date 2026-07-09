// Package claudecli wraps the `claude` command-line binary for one-shot,
// non-interactive completions.
//
// It runs `claude -p` (print mode) with the default Claude Code system prompt
// replaced by the caller's, so the request carries only the caller's tokens and
// authenticates with the machine's existing Claude login instead of an
// ANTHROPIC_API_KEY. Complete returns the model's raw reply; the internal/llm
// layer owns everything above it — the model to use, the timeout, and how to
// interpret the reply.
package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// binaryName is the CLI looked up on PATH.
const binaryName = "claude"

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

// Client is a one-shot wrapper around the `claude` CLI for a fixed model.
type Client struct {
	bin   string
	model string
	run   runnerFunc
}

// New builds a Client that calls the given model via the `claude` binary on
// PATH. Tests may override the bin and run fields directly.
func New(model string) *Client {
	return &Client{bin: binaryName, model: model, run: execRunner}
}

// Complete runs prompt with systemPrompt in one-shot mode and returns the
// model's raw reply. It honors ctx cancellation (including any deadline the
// caller set) but imposes no timeout of its own — that, like trimming and JSON
// decoding, is the caller's concern. An empty systemPrompt keeps Claude Code's
// default system prompt.
func (c *Client) Complete(ctx context.Context, systemPrompt, prompt string) (string, error) {
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
