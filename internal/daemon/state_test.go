package daemon

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestStatePath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")

	got := StatePath("/tmp/cc-tool-reviewer.sock")
	want := "/run/user/501/cc-tool-reviewer/tmp-cc-tool-reviewer.json"
	if got != want {
		t.Errorf("StatePath = %q, want %q", got, want)
	}

	// Same basename, different dirs: must not collide.
	if StatePath("/tmp/a.sock") == StatePath("/var/a.sock") {
		t.Error("state paths collide for distinct sockets")
	}
}

func TestIsChild(t *testing.T) {
	t.Setenv(childEnv, "")
	if IsChild() {
		t.Error("IsChild without env = true")
	}
	t.Setenv(childEnv, "1")
	if !IsChild() {
		t.Error("IsChild with env = false")
	}
}

func TestWriteReadState(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	socket := filepath.Join(t.TempDir(), "t.sock")

	if _, _, err := readState(socket); !os.IsNotExist(err) {
		t.Fatalf("readState with no file: err = %v, want not-exist", err)
	}

	st, err := WriteState(socket, "/tmp/x.log")
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Lock is held by this process: a fresh fd must see it.
	got, held, err := readState(socket)
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if !held {
		t.Error("lock not observed as held while owner is alive")
	}
	if got.PID != os.Getpid() || got.LogPath != "/tmp/x.log" || got.StartedAt.IsZero() {
		t.Errorf("state round-trip = %+v", got)
	}

	// A second claim on the same socket must fail while the lock is held.
	if _, err := WriteState(socket, "/tmp/y.log"); err == nil {
		t.Error("second WriteState succeeded; want lock conflict")
	}

	st.Remove()
	if _, _, err := readState(socket); !os.IsNotExist(err) {
		t.Errorf("state file still present after Remove: err = %v", err)
	}
}

func TestReadStateStale(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	socket := filepath.Join(t.TempDir(), "t.sock")
	// A state file with no lock holder — what a crashed daemon leaves.
	if err := os.MkdirAll(filepath.Dir(StatePath(socket)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(socket), []byte(`{"pid":4194000,"log_path":"/tmp/x.log"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, held, err := readState(socket)
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if held {
		t.Error("unlocked file observed as held")
	}
	if st.PID != 4194000 {
		t.Errorf("pid = %d", st.PID)
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

	if err := os.WriteFile(sock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if socketAlive(sock) {
		t.Error("expected false for stale socket file")
	}
}
