package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

type ReviewDecision struct {
	Decision string `json:"decision"` // "allow", "deny", "ask"
	Reason   string `json:"reason"`
}

type Reviewer struct {
	client       *anthropic.Client
	systemPrompt string
}

func NewReviewer(allowRules []string) *Reviewer {
	client := anthropic.NewClient()

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

	return &Reviewer{client: &client, systemPrompt: sb.String()}
}

func (r *Reviewer) Review(toolName string, toolInput json.RawMessage) (*ReviewDecision, error) {
	userMsg := fmt.Sprintf("Tool: %s\nInput: %s", toolName, string(toolInput))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 128,
		System: []anthropic.TextBlockParam{
			{
				Text: r.systemPrompt,
				CacheControl: anthropic.CacheControlEphemeralParam{
					Type: "ephemeral",
					TTL:  anthropic.CacheControlEphemeralTTLTTL1h,
				},
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic API error: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	text := resp.Content[0].Text

	// Strip markdown fences if the model wraps them anyway
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var decision ReviewDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		slog.Warn("failed to parse reviewer response", "text", text)
		return &ReviewDecision{Decision: "ask", Reason: "could not parse reviewer response"}, nil
	}

	// Normalize
	decision.Decision = strings.ToLower(strings.TrimSpace(decision.Decision))
	if decision.Decision != "allow" {
		decision.Decision = "ask"
	}

	return &decision, nil
}
