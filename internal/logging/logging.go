// Package logging configures the process-wide slog default.
//
// Output format follows the destination: a terminal gets colored,
// human-readable text; anything else (log file, pipe) gets JSON lines so
// the output stays machine-queryable with standard tools (grep, jq).
package logging

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// Setup installs the default slog handler, picking the format from what
// stderr is attached to.
func Setup() {
	slog.SetDefault(slog.New(NewHandler(os.Stderr, IsTerminal(os.Stderr))))
}

// NewHandler returns the handler for w: tint text when terminal is true,
// JSON lines otherwise.
func NewHandler(w io.Writer, terminal bool) slog.Handler {
	if terminal {
		return tint.NewHandler(w, &tint.Options{TimeFormat: time.Kitchen})
	}
	return slog.NewJSONHandler(w, nil)
}

// IsTerminal reports whether f is attached to a terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
