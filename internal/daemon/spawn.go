package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anish/cc-tool-reviewer/internal/paths"
)

// spawn re-executes the current binary in a new session, detached from the
// terminal, with stdout/stderr appended to logPath.
func spawn(logPath string) (pid int, err error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve executable: %w", err)
	}
	if err := paths.EnsureDir(filepath.Dir(logPath)); err != nil {
		return 0, fmt.Errorf("log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), childEnv+"=1")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	pid = cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}

// socketAlive reports whether a live process is serving socketPath. A stale
// socket file left by a crashed daemon refuses connections, so a successful
// dial means a running listener holds it.
func socketAlive(socketPath string) bool {
	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// processAlive reports whether pid exists (signal 0 probes without killing).
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}
