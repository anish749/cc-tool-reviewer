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

	// Build recent tool calls as separate lines
	var recent strings.Builder
	calls := ctx.RecentToolCalls
	if len(calls) > 5 {
		calls = calls[len(calls)-5:]
	}
	for _, tc := range calls {
		desc := tc.Description
		if desc == "" {
			desc = tc.Tool
		} else {
			desc = tc.Tool + " — " + desc
		}
		recent.WriteString("  · " + desc + "\n")
	}

	// Pack into the 4 args the Swift binary expects:
	// arg1: tool (with description)
	// arg2: command
	// arg3: ai reason
	// arg4: context block (user message + cwd + recent)
	toolDisplay := toolName
	if description != "" {
		toolDisplay = toolName + " — " + description
	}

	var context strings.Builder
	if cwd != "" {
		context.WriteString("📁 " + cwd + "\n\n")
	}
	if ctx.LastUserMessage != "" {
		context.WriteString(ctx.LastUserMessage)
	}
	if recent.Len() > 0 {
		if context.Len() > 0 {
			context.WriteString("\n\n")
		}
		context.WriteString("Recent:\n")
		context.WriteString(recent.String())
	}

	out, err := exec.Command(
		dialogBinary(),
		toolDisplay,
		truncate(command, 500),
		aiReason,
		context.String(),
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
