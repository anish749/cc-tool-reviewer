package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewHandlerJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, false))
	logger.Info("hello", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("non-terminal output is not JSON: %v\n%s", err, buf.String())
	}
	if entry["msg"] != "hello" || entry["key"] != "value" || entry["level"] != "INFO" {
		t.Errorf("unexpected fields: %v", entry)
	}
	if _, ok := entry["time"]; !ok {
		t.Errorf("missing time field: %v", entry)
	}
}

func TestNewHandlerTerminal(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, true))
	logger.Info("hello", "key", "value")

	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "value") {
		t.Errorf("terminal output missing message or attr: %q", out)
	}
	if json.Valid(buf.Bytes()) {
		t.Errorf("terminal output should be text, got JSON: %q", out)
	}
}
