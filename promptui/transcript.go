package promptui

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// transcriptEntry represents a single entry from the JSONL transcript.
type transcriptEntry struct {
	Message struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"message"`
}

// Context holds conversation context extracted from the transcript.
type Context struct {
	RecentUserMessages []string
	RecentToolCalls    []ToolCallSummary
}

// LastUserMessage returns the most recent user message, or empty string.
func (c Context) LastUserMessage() string {
	if len(c.RecentUserMessages) > 0 {
		return c.RecentUserMessages[len(c.RecentUserMessages)-1]
	}
	return ""
}

// ToolCallSummary is a brief description of a recent tool call.
type ToolCallSummary struct {
	Tool        string
	Description string
	DedupKey    string
}

// ReadContext reads the transcript JSONL file and extracts the last user message
// and recent tool call history.
func ReadContext(transcriptPath string, maxToolCalls int) Context {
	var ctx Context
	if transcriptPath == "" {
		return ctx
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		return ctx
	}
	defer f.Close()

	var entries []transcriptEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry transcriptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Walk backwards to find the last 3 user messages (actual text, not tool_result)
	for i := len(entries) - 1; i >= 0 && len(ctx.RecentUserMessages) < 3; i-- {
		e := entries[i]
		if e.Message.Role == "user" {
			text := extractUserText(e.Message.Content)
			if text != "" {
				ctx.RecentUserMessages = append(ctx.RecentUserMessages, text)
			}
		}
	}
	// Reverse to chronological order
	for i, j := 0, len(ctx.RecentUserMessages)-1; i < j; i, j = i+1, j-1 {
		ctx.RecentUserMessages[i], ctx.RecentUserMessages[j] = ctx.RecentUserMessages[j], ctx.RecentUserMessages[i]
	}

	// Collect recent tool calls from assistant messages, deduplicating
	seen := make(map[string]bool)
	for i := len(entries) - 1; i >= 0 && len(ctx.RecentToolCalls) < maxToolCalls; i-- {
		e := entries[i]
		if e.Message.Role != "assistant" {
			continue
		}
		calls := extractToolCalls(e.Message.Content)
		for j := len(calls) - 1; j >= 0 && len(ctx.RecentToolCalls) < maxToolCalls; j-- {
			key := calls[j].DedupKey
			if seen[key] {
				continue
			}
			seen[key] = true
			ctx.RecentToolCalls = append(ctx.RecentToolCalls, calls[j])
		}
	}

	// Reverse so they're in chronological order
	for i, j := 0, len(ctx.RecentToolCalls)-1; i < j; i, j = i+1, j-1 {
		ctx.RecentToolCalls[i], ctx.RecentToolCalls[j] = ctx.RecentToolCalls[j], ctx.RecentToolCalls[i]
	}

	return ctx
}

// extractUserText pulls plain text from a user message, skipping tool_result blocks.
func extractUserText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			m, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "tool_result" {
				continue
			}
			if t, ok := m["text"].(string); ok {
				parts = append(parts, strings.TrimSpace(t))
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// extractToolCalls pulls tool_use blocks from an assistant message.
func extractToolCalls(content any) []ToolCallSummary {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}

	var calls []ToolCallSummary
	for _, block := range blocks {
		m, ok := block.(map[string]any)
		if !ok || m["type"] != "tool_use" {
			continue
		}

		name, _ := m["name"].(string)
		desc := ""
		dedupKey := name
		if input, ok := m["input"].(map[string]any); ok {
			if fp, ok := input["file_path"].(string); ok {
				desc = fp
				dedupKey = "file:" + fp
			} else if cmd, ok := input["command"].(string); ok {
				if len(cmd) > 60 {
					desc = cmd[:60] + "..."
				} else {
					desc = cmd
				}
				dedupKey = "cmd:" + desc
				// Prefer description over raw command for display
				if d, ok := input["description"].(string); ok {
					desc = d
				}
			} else if d, ok := input["description"].(string); ok {
				desc = d
			}
		}

		calls = append(calls, ToolCallSummary{Tool: name, Description: desc, DedupKey: dedupKey})
	}
	return calls
}
