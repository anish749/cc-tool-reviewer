package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultLogFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	if got, want := DefaultLogFile(), "/xdg/state/cc-tool-reviewer/daemon.log"; got != want {
		t.Errorf("with XDG_STATE_HOME: %q, want %q", got, want)
	}

	t.Setenv("XDG_STATE_HOME", "")
	got := DefaultLogFile()
	home, _ := os.UserHomeDir()
	var want string
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Logs", "cc-tool-reviewer", "daemon.log")
	} else {
		want = filepath.Join(home, ".local", "state", "cc-tool-reviewer", "daemon.log")
	}
	if got != want {
		t.Errorf("platform default: %q, want %q", got, want)
	}
}

func TestRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
	if got, want := RuntimeDir(), "/run/user/501/cc-tool-reviewer"; got != want {
		t.Errorf("with XDG_RUNTIME_DIR: %q, want %q", got, want)
	}

	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := RuntimeDir(); !strings.HasSuffix(got, "cc-tool-reviewer") {
		t.Errorf("fallback should end in app dir: %q", got)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, err %v; want 0700", fi.Mode().Perm(), err)
	}
	// Idempotent on an existing private dir.
	if err := EnsureDir(dir); err != nil {
		t.Errorf("EnsureDir twice: %v", err)
	}

	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(loose); err == nil {
		t.Error("EnsureDir accepted a group/other-accessible dir")
	}
}
