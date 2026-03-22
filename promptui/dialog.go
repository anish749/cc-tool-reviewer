// Package promptui provides native macOS dialogs for tool call approval
// and AskUserQuestion responses.
// Uses a compiled Swift binary for the approval dialog (translucent HUD)
// and osascript for AskUserQuestion (list picker).
package promptui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApprovalResult is the outcome of a tool approval dialog.
// Decision represents the user's choice in an approval dialog.
type Decision int

const (
	DecisionApprove Decision = iota
	DecisionDeny
	DecisionLater // defer to Claude Code's terminal prompt
)

type ApprovalResult struct {
	Decision Decision
}

// dialogBinary returns the path to the compiled Swift approval dialog binary.
// It looks next to the main binary first, then falls back to the working directory.
func dialogBinary() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "approval-dialog")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "approval-dialog"
}

// ShowApproval shows a native macOS translucent HUD dialog for approving/denying a tool call.
// Uses the compiled Swift binary for the UI.
// "Later" defers the decision to Claude Code's normal terminal prompt.
func ShowApproval(toolName string, toolInput json.RawMessage, aiReason string, ctx Context) (ApprovalResult, error) {
	command := extractCommandSummary(toolName, toolInput)

	out, err := exec.Command(
		dialogBinary(),
		toolName,
		truncate(command, 500),
		aiReason,
		truncate(ctx.LastUserMessage, 200),
	).CombinedOutput()

	result := strings.TrimSpace(string(out))
	slog.Info("approval dialog", "output", result, "err", err)

	if err != nil {
		return ApprovalResult{Decision: DecisionLater}, nil
	}

	switch result {
	case "approve":
		return ApprovalResult{Decision: DecisionApprove}, nil
	case "deny":
		return ApprovalResult{Decision: DecisionDeny}, nil
	default:
		return ApprovalResult{Decision: DecisionLater}, nil
	}
}

// AskQuestionResult is the outcome of an AskUserQuestion dialog.
type AskQuestionResult struct {
	Cancelled bool
	Selected  string // the label of the selected option
}

// AskUserQuestionInput represents the parsed tool_input for AskUserQuestion.
type AskUserQuestionInput struct {
	Questions []struct {
		Question string `json:"question"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
}

// ShowAskUserQuestion shows a native macOS dialog for an AskUserQuestion tool call.
// Displays the question and options as a list the user can choose from.
func ShowAskUserQuestion(toolInput json.RawMessage, ctx Context) (AskQuestionResult, error) {
	var input AskUserQuestionInput
	if err := json.Unmarshal(toolInput, &input); err != nil {
		return AskQuestionResult{Cancelled: true}, err
	}

	if len(input.Questions) == 0 || len(input.Questions[0].Options) == 0 {
		return AskQuestionResult{Cancelled: true}, nil
	}

	q := input.Questions[0]

	// Build option labels with descriptions
	var optionLabels []string
	for _, opt := range q.Options {
		label := opt.Label
		if opt.Description != "" {
			label += " — " + opt.Description
		}
		optionLabels = append(optionLabels, quoteAppleScript(label))
	}

	prompt := q.Question
	if ctx.LastUserMessage != "" {
		prompt = "Context: " + truncate(ctx.LastUserMessage, 100) + "\n\n" + q.Question
	}

	script := fmt.Sprintf(
		`choose from list {%s} with title "cc-tool-reviewer" with prompt %s`,
		strings.Join(optionLabels, ", "),
		quoteAppleScript(prompt),
	)

	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	slog.Info("ask-question dialog", "output", strings.TrimSpace(string(out)), "err", err)
	if err != nil {
		return AskQuestionResult{Cancelled: true}, nil
	}

	selected := strings.TrimSpace(string(out))
	if selected == "false" || selected == "" {
		return AskQuestionResult{Cancelled: true}, nil
	}

	// Match back to the original label (strip description we appended)
	for _, opt := range q.Options {
		full := opt.Label
		if opt.Description != "" {
			full += " — " + opt.Description
		}
		if selected == full {
			return AskQuestionResult{Selected: opt.Label}, nil
		}
	}

	return AskQuestionResult{Selected: selected}, nil
}

func extractCommandSummary(toolName string, toolInput json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(toolInput, &m); err != nil {
		return string(toolInput)
	}

	switch toolName {
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	case "Edit", "Write", "Read":
		if fp, ok := m["file_path"].(string); ok {
			return fp
		}
	case "WebFetch":
		if url, ok := m["url"].(string); ok {
			return url
		}
	case "WebSearch":
		if q, ok := m["query"].(string); ok {
			return q
		}
	}
	return string(toolInput)
}

// quoteAppleScript quotes a string for use in AppleScript.
func quoteAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
