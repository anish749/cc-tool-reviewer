package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const DefaultDaemonLogPath = "/tmp/cc-tool-reviewer.log"

// daemonChildEnv marks the re-executed background child so it runs the
// server instead of daemonizing again.
const daemonChildEnv = "CC_TOOL_REVIEWER_DAEMON_CHILD"

// pidFilePath derives the pid file from the socket path so daemons on
// different sockets are tracked independently: /tmp/x.sock -> /tmp/x.pid.
func pidFilePath(socketPath string) string {
	return strings.TrimSuffix(socketPath, ".sock") + ".pid"
}

// runDaemonCommand handles `daemon <verb>` and returns the process exit
// code. In the re-executed background child, "start" returns -1: continue
// into main and run the server.
func runDaemonCommand(verb, socketPath, logPath string) int {
	switch verb {
	case "start":
		if os.Getenv(daemonChildEnv) != "" {
			return -1
		}
		return daemonStart(socketPath, logPath)
	case "stop":
		return daemonStop(socketPath)
	case "status":
		return daemonStatus(socketPath, logPath)
	default:
		fmt.Fprintln(os.Stderr, "usage: cc-tool-reviewer [flags] daemon {start|stop|status}")
		return 2
	}
}

func daemonStart(socketPath, logPath string) int {
	if socketAlive(socketPath) {
		fmt.Printf("daemon already running on %s\n", socketPath)
		return 0
	}
	pid, err := spawnDaemon(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}
	if err := os.WriteFile(pidFilePath(socketPath), []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: write pid file: %v\n", err)
	}
	if !waitFor(3*time.Second, func() bool { return socketAlive(socketPath) }) {
		fmt.Fprintf(os.Stderr, "daemon started (pid %d) but socket %s is not answering; check %s\n", pid, socketPath, logPath)
		return 1
	}
	fmt.Printf("daemon started (pid %d), socket %s, logs %s\n", pid, socketPath, logPath)
	return 0
}

func daemonStop(socketPath string) int {
	pidPath := pidFilePath(socketPath)
	pid, err := readPidFile(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("daemon not running (no pid file)")
			return 0
		}
		fmt.Fprintf(os.Stderr, "daemon stop: %v\n", err)
		return 1
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			os.Remove(pidPath)
			fmt.Printf("daemon not running (removed stale pid file for pid %d)\n", pid)
			return 0
		}
		fmt.Fprintf(os.Stderr, "daemon stop: signal pid %d: %v\n", pid, err)
		return 1
	}
	if !waitFor(5*time.Second, func() bool { return !processAlive(pid) }) {
		fmt.Fprintf(os.Stderr, "daemon (pid %d) did not exit within 5s\n", pid)
		return 1
	}
	os.Remove(pidPath)
	fmt.Printf("daemon stopped (pid %d)\n", pid)
	return 0
}

func daemonStatus(socketPath, logPath string) int {
	pid, pidErr := readPidFile(pidFilePath(socketPath))
	alive := socketAlive(socketPath)
	switch {
	case pidErr == nil && processAlive(pid):
		if !alive {
			fmt.Printf("daemon running (pid %d) but socket %s is not answering\n", pid, socketPath)
			return 1
		}
		fmt.Printf("daemon running (pid %d), socket %s, logs %s\n", pid, socketPath, logPath)
		return 0
	case alive:
		fmt.Printf("running on %s (foreground or unmanaged; no pid file)\n", socketPath)
		return 0
	default:
		fmt.Println("daemon not running")
		return 1
	}
}

// spawnDaemon re-executes the current binary in a new session, detached
// from the terminal, with stdout/stderr appended to logPath.
func spawnDaemon(logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve executable: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), daemonChildEnv+"=1")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	pid := cmd.Process.Pid
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

func readPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %s: %q", path, strings.TrimSpace(string(data)))
	}
	return pid, nil
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

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
