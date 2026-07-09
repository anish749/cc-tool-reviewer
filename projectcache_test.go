package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestProjectCacheConcurrentGet ensures concurrent Get calls don't race on
// the watched-dirs map. Run with `go test -race`.
func TestProjectCacheConcurrentGet(t *testing.T) {
	pc, err := NewProjectCache(time.Minute)
	if err != nil {
		t.Fatalf("NewProjectCache: %v", err)
	}
	defer pc.Stop()

	// Create a handful of project dirs with .claude/ so watchDir actually adds them.
	root := t.TempDir()
	const n = 8
	dirs := make([]string, n)
	for i := range dirs {
		d := filepath.Join(root, "p"+string(rune('a'+i)))
		if err := os.MkdirAll(filepath.Join(d, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		dirs[i] = d
	}

	var wg sync.WaitGroup
	for g := range 32 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := range 50 {
				_ = pc.Get(dirs[(g+i)%n])
			}
		}(g)
	}
	wg.Wait()
}
