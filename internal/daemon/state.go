package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anish/cc-tool-reviewer/internal/paths"
)

// childEnv marks the re-executed background child so it runs the server
// instead of daemonizing again.
const childEnv = "CC_TOOL_REVIEWER_DAEMON_CHILD"

// IsChild reports whether this process is the re-executed background child.
func IsChild() bool {
	return os.Getenv(childEnv) != ""
}

// State is the daemon's on-disk record: a JSON file holding pid, log path,
// and start time. The daemon process holds a flock on it for its lifetime,
// so lock state — not file existence — is the liveness truth: a crashed
// daemon leaves the file behind but never a held lock, and a pid can be
// recycled but a lock cannot.
type State struct {
	PID       int       `json:"pid"`
	LogPath   string    `json:"log_path"`
	StartedAt time.Time `json:"started_at"`

	file *os.File
	path string
}

// StatePath maps a socket path to its state file in the per-user runtime
// dir, so daemons on different sockets are tracked independently. The full
// (absolute) socket path is flattened into the name rather than hashed, to
// stay debuggable: /tmp/x.sock -> <runtime>/tmp-x.json.
func StatePath(socketPath string) string {
	abs, err := filepath.Abs(socketPath)
	if err != nil {
		abs = socketPath
	}
	name := strings.TrimPrefix(strings.TrimSuffix(abs, ".sock"), string(filepath.Separator))
	name = strings.ReplaceAll(name, string(filepath.Separator), "-")
	return filepath.Join(paths.RuntimeDir(), name+".json")
}

// WriteState claims the state file for this process: takes the exclusive
// lock (failing if another daemon holds it) and writes this process's
// record. The lock is held until Remove.
func WriteState(socketPath, logPath string) (*State, error) {
	path := StatePath(socketPath)
	if err := paths.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("runtime dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another daemon already holds %s", path)
	}
	st := &State{
		PID:       os.Getpid(),
		LogPath:   logPath,
		StartedAt: time.Now().UTC(),
		file:      f,
		path:      path,
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, fmt.Errorf("truncate state file: %w", err)
	}
	if err := json.NewEncoder(f).Encode(st); err != nil {
		f.Close()
		return nil, fmt.Errorf("write state file: %w", err)
	}
	return st, nil
}

// Remove deletes the state file and releases the lock.
func (st *State) Remove() {
	os.Remove(st.path)
	st.file.Close()
}

// readState reads the state file and probes its lock. held reports whether
// a live daemon holds it. A missing file returns os.ErrNotExist.
func readState(socketPath string) (st *State, held bool, err error) {
	f, err := os.Open(StatePath(socketPath))
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	held = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) != nil

	st = &State{}
	if err := json.NewDecoder(f).Decode(st); err != nil {
		return nil, held, fmt.Errorf("corrupt state file %s: %w", StatePath(socketPath), err)
	}
	return st, held, nil
}
