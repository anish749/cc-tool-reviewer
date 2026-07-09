package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

// EnvUseCLIClient, when set to a non-empty value, routes review through the
// local `claude` CLI (using the machine's Claude login) instead of the
// Anthropic SDK (which authenticates with ANTHROPIC_API_KEY).
const EnvUseCLIClient = "USE_CLAUDE_CLI_CLIENT"

const reviewModel = "claude-haiku-4-5"

type ReviewDecision struct {
	Decision string `json:"decision"` // "allow", "deny", "ask"
	Reason   string `json:"reason"`
}

// Reviewer decides whether a tool call that missed the permission rules should
// be allowed or escalated to the user. It owns the reviewer-specific prompt and
// decision logic; the LLM call is an injected dependency.
type Reviewer struct {
	llm          llm.Client
	systemPrompt string
}

// NewReviewer builds a reviewer backed by the given LLM client.
func NewReviewer(client llm.Client, allowRules []string) *Reviewer {
	return &Reviewer{llm: client, systemPrompt: buildSystemPrompt(allowRules)}
}

// NewReviewLLM selects the LLM client from the environment: the CLI-backed
// client when USE_CLAUDE_CLI_CLIENT is set, otherwise the Anthropic SDK one.
func NewReviewLLM() llm.Client {
	opts := []llm.Option{llm.WithModel(reviewModel)}
	if os.Getenv(EnvUseCLIClient) != "" {
		slog.Info("reviewer using claude CLI client")
		return llm.NewCLIClient(opts...)
	}
	return llm.NewAnthropicClient(opts...)
}

// Review asks the model to classify a tool call. It returns an "allow"/"ask"
// decision, coercing anything unclear to "ask". A malformed model reply is a
// soft "ask" (the caller can still surface a dialog); a transport failure is a
// hard error the caller decides how to handle.
func (r *Reviewer) Review(toolName string, toolInput json.RawMessage) (*ReviewDecision, error) {
	var decision ReviewDecision
	err := r.llm.JSON(context.Background(), r.systemPrompt, buildUserMessage(toolName, toolInput), &decision)
	if err != nil {
		var parseErr *llm.ParseError
		if errors.As(err, &parseErr) {
			slog.Warn("failed to parse reviewer response", "text", parseErr.Raw)
			return &ReviewDecision{Decision: "ask", Reason: "could not parse reviewer response"}, nil
		}
		return nil, fmt.Errorf("review: %w", err)
	}

	decision.Decision = strings.ToLower(strings.TrimSpace(decision.Decision))
	if decision.Decision != "allow" {
		slog.Info("reviewer falling back to ask", "model_decision", decision.Decision, "reason", decision.Reason)
		decision.Decision = "ask"
	}
	return &decision, nil
}

// buildSystemPrompt renders the reviewer instructions with the user's allowed
// patterns embedded.
func buildSystemPrompt(allowRules []string) string {
	var sb strings.Builder
	sb.WriteString(`You are reviewing tool calls for a CLI tool called Claude Code. A tool call is about to execute that did not exactly match the user's configured permission rules. Your job is to reduce unnecessary prompts by allowing commands that are consistent with what the user has already permitted.

The user has explicitly allowed the following patterns:
`)
	for _, rule := range allowRules {
		sb.WriteString("- ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}
	sb.WriteString(`
Default to "allow". Only respond "ask" if you cannot find any reasonable connection between the command and what the user has already allowed.

A command should be allowed if ANY of these are true:
- It is a composition of allowed commands (pipes, &&, ||, ;, subshells, multi-line scripts)
- It is a variation of an allowed pattern (different flags, arguments, or targets)
- It is a read-only command that merely observes state (pgrep, whoami, which, ps, lsof, date, wc, du, df, uptime, file, type, env, printenv, id, hostname, uname, sw_vers, etc.)
- It is a standard development command that a developer using the allowed tools would reasonably also use

Only evaluate commands actually EXECUTED by the shell, not strings inside quotes, echo arguments, or data literals.

Respond with ONLY a valid JSON object. No markdown, no explanation, no code fences:
{"decision": "allow" or "ask", "reason": "brief one-line reason"}`)
	return sb.String()
}

// buildUserMessage is the per-call user turn describing the tool invocation.
func buildUserMessage(toolName string, toolInput json.RawMessage) string {
	return fmt.Sprintf("Tool: %s\nInput: %s", toolName, string(toolInput))
}
