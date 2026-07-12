package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestPidFilePath(t *testing.T) {
	cases := map[string]string{
		"/tmp/cc-tool-reviewer.sock": "/tmp/cc-tool-reviewer.pid",
		"/tmp/custom":                "/tmp/custom.pid",
	}
	for socket, want := range cases {
		if got := pidFilePath(socket); got != want {
			t.Errorf("pidFilePath(%q) = %q, want %q", socket, got, want)
		}
	}
}

func TestSocketAlive(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "t.sock")

	if socketAlive(sock) {
		t.Error("expected false for missing socket")
	}

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if !socketAlive(sock) {
		t.Error("expected true for live listener")
	}
	l.Close()

	// Simulate a stale socket file left by a crashed daemon.
	if err := os.WriteFile(sock, nil, 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if socketAlive(sock) {
		t.Error("expected false for stale socket file")
	}
}

func TestReadPidFile(t *testing.T) {
	dir := t.TempDir()

	if _, err := readPidFile(filepath.Join(dir, "missing.pid")); !os.IsNotExist(err) {
		t.Errorf("expected not-exist error, got %v", err)
	}

	good := filepath.Join(dir, "good.pid")
	os.WriteFile(good, []byte("1234\n"), 0o644)
	if pid, err := readPidFile(good); err != nil || pid != 1234 {
		t.Errorf("readPidFile = %d, %v; want 1234, nil", pid, err)
	}

	bad := filepath.Join(dir, "bad.pid")
	os.WriteFile(bad, []byte("not-a-pid\n"), 0o644)
	if _, err := readPidFile(bad); err == nil {
		t.Error("expected error for garbage pid file")
	}
}

// TestDaemonLifecycle builds the binary and exercises
// daemon start -> status -> stop end to end.
func TestDaemonLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping daemon lifecycle test in short mode")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "cc-tool-reviewer")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sock := filepath.Join(dir, "d.sock")
	logf := filepath.Join(dir, "d.log")
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, append([]string{"--socket", sock, "--log-file", logf}, args...)...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run("daemon", "start")
	if err != nil {
		t.Fatalf("daemon start: %v\n%s", err, out)
	}
	if !strings.Contains(out, "daemon started") {
		t.Errorf("start output = %q, want it to contain %q", out, "daemon started")
	}

	pid, err := readPidFile(pidFilePath(sock))
	if err != nil {
		t.Fatalf("read pid file after start: %v", err)
	}
	t.Cleanup(func() {
		if processAlive(pid) {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	if !socketAlive(sock) {
		t.Fatal("socket not answering after start")
	}
	if !processAlive(pid) {
		t.Fatalf("daemon process %d not alive after start", pid)
	}

	// Second start is a no-op.
	out, err = run("daemon", "start")
	if err != nil || !strings.Contains(out, "already running") {
		t.Errorf("second start = %q, %v; want 'already running', nil", out, err)
	}
	if again, _ := readPidFile(pidFilePath(sock)); again != pid {
		t.Errorf("second start rewrote pid file: %d -> %d", pid, again)
	}

	out, err = run("daemon", "status")
	if err != nil || !strings.Contains(out, "daemon running (pid "+strconv.Itoa(pid)+")") {
		t.Errorf("status = %q, %v; want running with pid %d, nil", out, err, pid)
	}

	out, err = run("daemon", "stop")
	if err != nil {
		t.Fatalf("daemon stop: %v\n%s", err, out)
	}
	if processAlive(pid) {
		t.Errorf("daemon process %d still alive after stop", pid)
	}
	if _, err := os.Stat(pidFilePath(sock)); !os.IsNotExist(err) {
		t.Errorf("pid file still present after stop: %v", err)
	}

	// Stop is idempotent.
	out, err = run("daemon", "stop")
	if err != nil || !strings.Contains(out, "not running") {
		t.Errorf("second stop = %q, %v; want 'not running', nil", out, err)
	}

	if logData, err := os.ReadFile(logf); err != nil || !strings.Contains(string(logData), "listening") {
		t.Errorf("log file should contain 'listening' (err %v):\n%s", err, logData)
	}
}
