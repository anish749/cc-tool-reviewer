package main

import (
	"encoding/json"
	"log/slog"
	"net/url"
)

type autoAllowHandler struct {
	logAttrs func(input json.RawMessage) []slog.Attr
}

var autoAllowTools = map[string]autoAllowHandler{
	"WebFetch":  {logAttrs: webFetchAttrs},
	"WebSearch": {logAttrs: webSearchAttrs},
}

// AutoAllow checks if a tool should be auto-approved and logs the relevant details.
// Returns true if the tool was handled (caller should write allow and return).
func AutoAllow(toolName string, toolInput json.RawMessage) bool {
	handler, ok := autoAllowTools[toolName]
	if !ok {
		return false
	}

	attrs := handler.logAttrs(toolInput)
	args := make([]any, 0, len(attrs)*2+2)
	args = append(args, "tool", toolName)
	for _, a := range attrs {
		args = append(args, a.Key, a.Value.String())
	}
	slog.Info("auto-allowed", args...)

	return true
}

func webFetchAttrs(input json.RawMessage) []slog.Attr {
	var params struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	json.Unmarshal(input, &params)

	host := ""
	if u, err := url.Parse(params.URL); err == nil {
		host = u.Host
	}

	return []slog.Attr{
		slog.String("url", params.URL),
		slog.String("host", host),
		slog.String("prompt", truncate(params.Prompt, 80)),
	}
}

func webSearchAttrs(input json.RawMessage) []slog.Attr {
	var params struct {
		Query string `json:"query"`
	}
	json.Unmarshal(input, &params)

	return []slog.Attr{
		slog.String("query", params.Query),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
