// Package daemon manages the background lifecycle of cc-tool-reviewer:
// starting a detached copy of the process, stopping it, and reporting its
// status. Liveness is anchored on a flock-held state file (see State), not
// on pid files alone, so stale files and recycled pids cannot mislead stop.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// DefaultLogPath is where the background process's output goes unless
// overridden with --log-file.
const DefaultLogPath = "/tmp/cc-tool-reviewer.log"

// Run executes a daemon verb and returns the process exit code.
func Run(verb, socketPath, logPath string) int {
	switch verb {
	case "start":
		return start(socketPath, logPath)
	case "stop":
		return stop(socketPath)
	case "status":
		return status(socketPath)
	default:
		fmt.Fprintln(os.Stderr, "usage: cc-tool-reviewer [flags] daemon {start|stop|status}")
		return 2
	}
}

func start(socketPath, logPath string) int {
	if socketAlive(socketPath) {
		fmt.Printf("daemon already running on %s\n", socketPath)
		return 0
	}
	pid, err := spawn(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon start: %v\n", err)
		return 1
	}
	if !waitFor(3*time.Second, func() bool { return socketAlive(socketPath) }) {
		fmt.Fprintf(os.Stderr, "daemon started (pid %d) but socket %s is not answering; check %s\n", pid, socketPath, logPath)
		return 1
	}
	fmt.Printf("daemon started (pid %d), socket %s, logs %s\n", pid, socketPath, logPath)
	return 0
}

func stop(socketPath string) int {
	st, held, err := readState(socketPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		fmt.Println("daemon not running")
		return 0
	case err != nil && !held:
		// Corrupt but unlocked: no live owner, safe to clear.
		os.Remove(StatePath(socketPath))
		fmt.Println("daemon not running (cleared corrupt state file)")
		return 0
	case err != nil:
		fmt.Fprintf(os.Stderr, "daemon stop: %v\n", err)
		return 1
	case !held:
		os.Remove(StatePath(socketPath))
		fmt.Printf("daemon not running (cleared stale state file for pid %d)\n", st.PID)
		return 0
	}

	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "daemon stop: signal pid %d: %v\n", st.PID, err)
		return 1
	}
	if !waitFor(5*time.Second, func() bool { return !processAlive(st.PID) }) {
		fmt.Fprintf(os.Stderr, "daemon (pid %d) did not exit within 5s\n", st.PID)
		return 1
	}
	os.Remove(StatePath(socketPath))
	fmt.Printf("daemon stopped (pid %d)\n", st.PID)
	return 0
}

func status(socketPath string) int {
	st, held, err := readState(socketPath)
	switch {
	case err == nil && held:
		if !socketAlive(socketPath) {
			fmt.Printf("daemon running (pid %d) but socket %s is not answering\n", st.PID, socketPath)
			return 1
		}
		fmt.Printf("daemon running (pid %d), socket %s, logs %s\n", st.PID, socketPath, st.LogPath)
		return 0
	case socketAlive(socketPath):
		fmt.Printf("running on %s (unmanaged; started in foreground?)\n", socketPath)
		return 0
	default:
		fmt.Println("daemon not running")
		return 1
	}
}
