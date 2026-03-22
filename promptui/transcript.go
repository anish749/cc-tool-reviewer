package promptui

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// TranscriptEntry represents a single entry from the JSONL transcript.
type TranscriptEntry struct {
	Role    string `json:"role"`
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"message"`
}

// Context holds conversation context extracted from the transcript.
type Context struct {
	LastUserMessage string
	RecentMessages  []string // last N messages summarized as "role: text"
}

// ReadContext reads the transcript JSONL file and extracts the last user message
// and recent conversation context.
func ReadContext(transcriptPath string, maxMessages int) Context {
	var ctx Context
	if transcriptPath == "" {
		return ctx
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		return ctx
	}
	defer f.Close()

	var entries []TranscriptEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for long lines
	for scanner.Scan() {
		var entry TranscriptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Walk backwards to find the last user message
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Message.Role == "user" {
			ctx.LastUserMessage = extractText(e.Message.Content)
			if ctx.LastUserMessage != "" {
				break
			}
		}
	}

	// Collect recent messages
	start := len(entries) - maxMessages
	if start < 0 {
		start = 0
	}
	for _, e := range entries[start:] {
		role := e.Message.Role
		if role == "" {
			continue
		}
		text := extractText(e.Message.Content)
		if text == "" {
			continue
		}
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		ctx.RecentMessages = append(ctx.RecentMessages, role+": "+text)
	}

	return ctx
}

// extractText pulls plain text from a message content field,
// which can be a string or an array of content blocks.
func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, strings.TrimSpace(t))
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}
