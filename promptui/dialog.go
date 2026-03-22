// Package promptui provides native macOS dialogs for tool call approval
// and AskUserQuestion responses, using osascript.
package promptui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// ApprovalResult is the outcome of a tool approval dialog.
type ApprovalResult struct {
	Approved bool
}

// ShowApproval shows a native macOS dialog for approving/denying a tool call.
// It displays the tool name, command details, AI reason, and conversation context.
func ShowApproval(toolName string, toolInput json.RawMessage, aiReason string, ctx Context) (ApprovalResult, error) {
	command := extractCommandSummary(toolName, toolInput)

	var body strings.Builder
	if ctx.LastUserMessage != "" {
		body.WriteString("User asked: ")
		body.WriteString(truncate(ctx.LastUserMessage, 150))
		body.WriteString("\n\n")
	}
	body.WriteString("Tool: ")
	body.WriteString(toolName)
	body.WriteString("\nCommand: ")
	body.WriteString(truncate(command, 300))
	if aiReason != "" {
		body.WriteString("\n\nAI reason: ")
		body.WriteString(aiReason)
	}

	script := fmt.Sprintf(
		`display dialog %s with title "cc-tool-reviewer" buttons {"Deny", "Approve"} default button "Approve"`,
		quoteAppleScript(body.String()),
	)

	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	slog.Info("approval dialog", "output", strings.TrimSpace(string(out)), "err", err)
	if err != nil {
		return ApprovalResult{Approved: false}, nil
	}

	return ApprovalResult{Approved: strings.Contains(string(out), "Approve")}, nil
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
