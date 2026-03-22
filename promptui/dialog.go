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
