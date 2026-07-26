package reviewlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLog_UnmatchedField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.jsonl")
	l := New(path)

	l.Log("Bash", json.RawMessage(`{"command":"which node 2>&1"}`), "allow",
		"read-only lookup", []string{"which node"})
	l.Log("Bash", json.RawMessage(`{"command":"docker build ."}`), "ask",
		"builds an image", nil)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	var e Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Unmatched) != 1 || e.Unmatched[0] != "which node" {
		t.Errorf("unmatched = %q, want [\"which node\"]", e.Unmatched)
	}

	// nil unmatched is omitted from the JSON entirely
	if strings.Contains(lines[1], "unmatched") {
		t.Errorf("entry with nil unmatched should omit the field: %s", lines[1])
	}
}
