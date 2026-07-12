package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestLifecycle builds the real binary and exercises
// daemon start -> status -> stop end to end.
func TestLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping daemon lifecycle e2e in short mode")
	}

	// Unix socket paths are capped at ~104 bytes on macOS; t.TempDir() is
	// too deep, so use a short-lived dir directly under /tmp.
	dir, err := os.MkdirTemp("/tmp", "ccd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Keep daemon state out of the real runtime dir — for both the test
	// process (readState below) and the spawned binary (run below).
	t.Setenv("XDG_RUNTIME_DIR", dir)

	// Not named cc-tool-reviewer: XDG_RUNTIME_DIR points at this same dir,
	// so RuntimeDir() would collide with a binary of that name.
	bin := filepath.Join(dir, "reviewer-under-test")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/anish/cc-tool-reviewer").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sock := filepath.Join(dir, "d.sock")
	logf := filepath.Join(dir, "d.log")
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, append([]string{"--socket", sock, "--log-file", logf}, args...)...)
		// The spawned daemon must pass startup validation without real
		// credentials being present (or absent) on the host running tests.
		cmd.Env = append(os.Environ(),
			"ANTHROPIC_API_KEY=test-dummy",
			"USE_CLAUDE_CLI_CLIENT=",
			"XDG_RUNTIME_DIR="+dir,
		)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run("daemon", "start")
	if err != nil {
		logData, _ := os.ReadFile(logf)
		t.Fatalf("daemon start: %v\n%s\n--- daemon log ---\n%s", err, out, logData)
	}
	if !strings.Contains(out, "daemon started") {
		t.Errorf("start output = %q", out)
	}

	st, held, err := readState(sock)
	if err != nil || !held {
		t.Fatalf("state after start: held=%v err=%v", held, err)
	}
	t.Cleanup(func() {
		if processAlive(st.PID) {
			syscall.Kill(st.PID, syscall.SIGKILL)
		}
	})
	if st.LogPath != logf {
		t.Errorf("state log path = %q, want %q", st.LogPath, logf)
	}

	out, err = run("daemon", "start")
	if err != nil || !strings.Contains(out, "already running") {
		t.Errorf("second start = %q, %v; want 'already running', nil", out, err)
	}

	out, err = run("daemon", "status")
	if err != nil || !strings.Contains(out, "daemon running") {
		t.Errorf("status = %q, %v; want running, nil", out, err)
	}

	out, err = run("daemon", "stop")
	if err != nil {
		t.Fatalf("daemon stop: %v\n%s", err, out)
	}
	if processAlive(st.PID) {
		t.Errorf("daemon process %d still alive after stop", st.PID)
	}
	if _, _, err := readState(sock); !os.IsNotExist(err) {
		t.Errorf("state file still present after stop: %v", err)
	}

	// Stop and status are safe when nothing is running.
	if out, err = run("daemon", "stop"); err != nil || !strings.Contains(out, "not running") {
		t.Errorf("second stop = %q, %v; want 'not running', nil", out, err)
	}
	if out, _ = run("daemon", "status"); !strings.Contains(out, "not running") {
		t.Errorf("status when stopped = %q; want 'not running'", out)
	}

	// A crashed daemon's leftover state file must not block the next start.
	if err := os.WriteFile(StatePath(sock), []byte(`{"pid":4194000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run("daemon", "start")
	if err != nil {
		t.Fatalf("start over stale state: %v\n%s", err, out)
	}
	st2, _, err := readState(sock)
	if err != nil {
		t.Fatalf("state after restart: %v", err)
	}
	if out, err = run("daemon", "stop"); err != nil {
		t.Fatalf("final stop (pid %d): %v\n%s", st2.PID, err, out)
	}

	logData, err := os.ReadFile(logf)
	if err != nil || !strings.Contains(string(logData), "listening") {
		t.Errorf("log file should contain 'listening' (err %v):\n%s", err, logData)
	}
	// JSON log lines in the file, per internal/logging.
	if !strings.Contains(string(logData), `"msg":"listening"`) {
		t.Errorf("log file should be JSON lines:\n%s", logData)
	}
}
