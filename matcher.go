package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Rule represents a parsed permission rule.
type Rule struct {
	Tool    string
	Pattern string // raw inner pattern, e.g. "rg:*", "git status *", "*", ""
}

var ruleRe = regexp.MustCompile(`^(\w+)\((.+)\)$`)

func ParseRule(raw string) (Rule, bool) {
	m := ruleRe.FindStringSubmatch(raw)
	if m != nil {
		return Rule{Tool: m[1], Pattern: m[2]}, true
	}
	// Bare tool name like "WebSearch"
	if raw != "" && !strings.Contains(raw, "(") {
		return Rule{Tool: raw, Pattern: "*"}, true
	}
	return Rule{}, false
}

// ToolInputString extracts the matchable string from a tool call.
func ToolInputString(toolName string, toolInput json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(toolInput, &m); err != nil {
		return ""
	}

	switch toolName {
	case "Bash":
		if v, ok := m["command"].(string); ok {
			return v
		}
	case "Read", "Edit", "Write":
		if v, ok := m["file_path"].(string); ok {
			return v
		}
	case "Glob":
		if v, ok := m["pattern"].(string); ok {
			return v
		}
	case "Grep":
		if v, ok := m["pattern"].(string); ok {
			return v
		}
	case "WebFetch":
		if v, ok := m["url"].(string); ok {
			return v
		}
	case "WebSearch":
		if v, ok := m["query"].(string); ok {
			return v
		}
	default:
		return string(toolInput)
	}
	return ""
}

func matchPattern(input, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// "rg:*" → input starts with "rg" (alone or followed by space+args)
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return input == prefix || strings.HasPrefix(input, prefix+" ")
	}

	// "git status *" → input starts with "git status" (alone or with args)
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return input == prefix || strings.HasPrefix(input, prefix+" ")
	}

	// Path glob with ** (e.g. "~/go/pkg/mod/**")
	if strings.Contains(pattern, "**") {
		expanded := expandTilde(pattern)
		// Convert ** glob to prefix match
		prefix := strings.Split(expanded, "**")[0]
		return strings.HasPrefix(input, prefix)
	}

	// "domain:*" style for WebFetch — treat same as prefix:*
	if strings.Contains(pattern, ":*") {
		// Already handled above; this catches mid-string :*
		idx := strings.Index(pattern, ":*")
		prefix := pattern[:idx]
		return strings.Contains(input, prefix)
	}

	// Wildcard at end: "git branch -d*" → prefix match
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(input, prefix)
	}

	// Exact match
	return input == pattern
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// AlwaysAllow returns true for tool types that should be auto-approved without
// any matching or AI review.
func AlwaysAllow(toolName string) bool {
	switch toolName {
	case "WebFetch", "WebSearch":
		return true
	}
	return false
}

// isCompoundCommand checks if a bash command is compound (multi-line, chained,
// backgrounded, uses subshells, etc.) and shouldn't be matched by simple prefix rules.
func isCompoundCommand(cmd string) bool {
	return strings.ContainsAny(cmd, "\n;") ||
		strings.Contains(cmd, "&&") ||
		strings.Contains(cmd, "||") ||
		strings.Contains(cmd, "$(") ||
		strings.ContainsRune(cmd, '`')
}

// MatchesAny checks if a tool call matches any rule in the list.
func MatchesAny(toolName string, toolInput json.RawMessage, rules []Rule) bool {
	input := ToolInputString(toolName, toolInput)

	// Compound bash commands should not match simple prefix rules — send to AI
	if toolName == "Bash" && isCompoundCommand(input) {
		return false
	}

	for _, r := range rules {
		if r.Tool != toolName {
			continue
		}
		if matchPattern(input, r.Pattern) {
			return true
		}
	}
	return false
}
