package reviewlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogIncludesUnmatchedCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	logger := New(path)
	logger.Log(
		"Bash",
		json.RawMessage(`{"command":"git status && mystery --flag"}`),
		[]string{"mystery --flag"},
		"allow",
		"safe command",
	)
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(entry.UnmatchedCommands) != 1 || entry.UnmatchedCommands[0] != "mystery --flag" {
		t.Fatalf("UnmatchedCommands = %#v, want [\"mystery --flag\"]", entry.UnmatchedCommands)
	}
}
