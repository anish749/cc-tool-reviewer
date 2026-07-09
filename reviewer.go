package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// EnvUseCLIClient, when set to a non-empty value, routes AI review through the
// local `claude` CLI (using the machine's Claude login) instead of the
// Anthropic SDK (which authenticates with ANTHROPIC_API_KEY).
const EnvUseCLIClient = "USE_CLAUDE_CLI_CLIENT"

type ReviewDecision struct {
	Decision string `json:"decision"` // "allow", "deny", "ask"
	Reason   string `json:"reason"`
}

// Reviewer decides whether a tool call that missed the permission rules should
// be allowed or escalated to the user. Implementations differ only in how they
// reach the model; both share the prompt and response contract defined here.
type Reviewer interface {
	Review(toolName string, toolInput json.RawMessage) (*ReviewDecision, error)
}

// NewReviewer builds the reviewer selected by the environment: the CLI-backed
// reviewer when USE_CLAUDE_CLI_CLIENT is set, otherwise the Anthropic SDK one.
func NewReviewer(allowRules []string) Reviewer {
	systemPrompt := buildSystemPrompt(allowRules)
	if os.Getenv(EnvUseCLIClient) != "" {
		slog.Info("reviewer using claude CLI client")
		return newCLIReviewer(systemPrompt)
	}
	return newAPIReviewer(systemPrompt)
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

// parseReviewResponse turns the model's raw reply into a decision. Anything it
// cannot parse, or any decision other than "allow", becomes "ask" so unclear
// cases escalate to the user rather than silently passing.
func parseReviewResponse(text string) *ReviewDecision {
	text = stripCodeFences(text)

	var decision ReviewDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		slog.Warn("failed to parse reviewer response", "text", text)
		return &ReviewDecision{Decision: "ask", Reason: "could not parse reviewer response"}
	}

	decision.Decision = strings.ToLower(strings.TrimSpace(decision.Decision))
	if decision.Decision != "allow" {
		decision.Decision = "ask"
	}
	return &decision
}

// stripCodeFences removes a leading ```json / ``` fence and a trailing ``` that
// the model sometimes wraps around its JSON reply.
func stripCodeFences(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}
