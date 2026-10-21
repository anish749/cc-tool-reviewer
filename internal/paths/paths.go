// Package paths resolves where cc-tool-reviewer keeps its files, following
// the XDG Base Directory spec with platform-native fallbacks:
//
//   - logs: $XDG_STATE_HOME/cc-tool-reviewer/ when set (the spec names logs
//     as canonical state data); otherwise ~/Library/Logs/cc-tool-reviewer/
//     on macOS and ~/.local/state/cc-tool-reviewer/ elsewhere.
//   - runtime files (daemon state): $XDG_RUNTIME_DIR/cc-tool-reviewer/ when
//     set; otherwise a subdirectory of the per-user temp dir ($TMPDIR on
//     macOS is per-user and 0700 already).
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

const appName = "cc-tool-reviewer"

// DefaultLogFile is where the daemon's log goes unless --log-file overrides.
func DefaultLogFile() string {
	return filepath.Join(stateDir(), "daemon.log")
}

func stateDir() string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return filepath.Join(s, appName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), appName)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", appName)
	}
	return filepath.Join(home, ".local", "state", appName)
}

// RuntimeDir is where per-user runtime files (daemon state) live.
func RuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, appName)
	}
	return filepath.Join(os.TempDir(), appName)
}

// EnsureDir creates dir (0700) and verifies it is a directory owned by the
// current user with no group/other access. The check matters when the
// parent is a shared location like /tmp, where a pre-created directory
// could belong to someone else.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s has group/other access (%v); refusing to use it", dir, fi.Mode().Perm())
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("%s is owned by uid %d, not us (uid %d)", dir, st.Uid, os.Geteuid())
	}
	return nil
}
