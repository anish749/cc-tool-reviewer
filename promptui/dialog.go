// Package promptui provides native macOS dialogs for tool call approval.
// Uses a compiled Swift binary for a translucent HUD overlay.
package promptui

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Decision represents the user's choice in an approval dialog.
type Decision int

const (
	DecisionApprove Decision = iota
	DecisionDeny
	DecisionLater // defer to Claude Code's terminal prompt
)

type ApprovalResult struct {
	Decision Decision
	Feedback string // optional user feedback text
}

// dialogBinary returns the path to the compiled Swift approval dialog binary.
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
// Passes context as CLI args to match the Swift binary's expected interface.
func ShowApproval(toolName string, toolInput json.RawMessage, aiReason string, cwd string, ctx Context) (ApprovalResult, error) {
	command := extractCommandSummary(toolName, toolInput)
	description := extractDescription(toolInput)

	// Build a rich user message with all context
	var userMsg strings.Builder
	if ctx.LastUserMessage != "" {
		userMsg.WriteString(ctx.LastUserMessage)
	}
	if cwd != "" {
		if userMsg.Len() > 0 {
			userMsg.WriteString("\n\n")
		}
		userMsg.WriteString("📁 " + cwd)
	}
	if len(ctx.RecentToolCalls) > 0 {
		if userMsg.Len() > 0 {
			userMsg.WriteString("\n\n")
		}
		userMsg.WriteString("Recent: ")
		for i, tc := range ctx.RecentToolCalls {
			if i > 0 {
				userMsg.WriteString(" → ")
			}
			if tc.Description != "" {
				userMsg.WriteString(tc.Description)
			} else {
				userMsg.WriteString(tc.Tool)
			}
		}
	}

	// If there's a description, prepend it to the tool name
	toolDisplay := toolName
	if description != "" {
		toolDisplay = toolName + " — " + description
	}

	out, err := exec.Command(
		dialogBinary(),
		toolDisplay,
		truncate(command, 500),
		aiReason,
		userMsg.String(),
	).CombinedOutput()

	output := strings.TrimSpace(string(out))
	slog.Info("approval dialog", "output", output, "err", err)

	if err != nil {
		return ApprovalResult{Decision: DecisionLater}, nil
	}

	// Output format: "decision\nfeedback" (feedback is optional)
	lines := strings.SplitN(output, "\n", 2)
	decision := lines[0]
	feedback := ""
	if len(lines) > 1 {
		feedback = strings.TrimSpace(lines[1])
	}

	switch decision {
	case "approve":
		return ApprovalResult{Decision: DecisionApprove, Feedback: feedback}, nil
	case "deny":
		return ApprovalResult{Decision: DecisionDeny, Feedback: feedback}, nil
	default:
		return ApprovalResult{Decision: DecisionLater, Feedback: feedback}, nil
	}
}

func extractDescription(toolInput json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(toolInput, &m); err != nil {
		return ""
	}
	if d, ok := m["description"].(string); ok {
		return d
	}
	return ""
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
