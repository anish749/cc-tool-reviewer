package reviewlog

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxSizeMB  = 50
	maxBackups = 10
)

// Entry is a single JSONL record written to the review log.
type Entry struct {
	Timestamp string          `json:"ts"`
	ToolName  string          `json:"tool"`
	ToolInput json.RawMessage `json:"input"`
	Decision  string          `json:"decision"`
	Reason    string          `json:"reason"`
	// Unmatched lists the sub-commands that matched no allow rule —
	// why the call escalated past the static check to AI review.
	Unmatched []string `json:"unmatched,omitempty"`
}

// Logger writes JSONL review log entries to a rotating file.
type Logger struct {
	mu sync.Mutex
	w  io.WriteCloser
}

// New creates a Logger that writes to the given path with rotation.
// Returns nil if path is empty (no-op logger).
func New(path string) *Logger {
	if path == "" {
		return nil
	}
	return &Logger{
		w: &lumberjack.Logger{
			Filename:   path,
			MaxSize:    maxSizeMB,
			MaxBackups: maxBackups,
			Compress:   true,
		},
	}
}

// Log appends a review entry. Safe for concurrent use. No-op on nil receiver.
func (l *Logger) Log(toolName string, toolInput json.RawMessage, decision, reason string, unmatched []string) {
	if l == nil {
		return
	}
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ToolName:  toolName,
		ToolInput: toolInput,
		Decision:  decision,
		Reason:    reason,
		Unmatched: unmatched,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		slog.Error("reviewlog: marshal failed", "err", err)
		return
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.w.Write(line); err != nil {
		slog.Error("reviewlog: write failed", "err", err)
	}
}

// Close closes the underlying writer. No-op on nil receiver.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	return l.w.Close()
}
