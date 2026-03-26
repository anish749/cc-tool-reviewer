// Package configwatcher watches Claude Code settings directories for changes
// and triggers a reload callback when .json files are created, modified, or deleted.
package configwatcher

import (
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a directory for settings file changes and triggers a callback.
type Watcher struct {
	fsw      *fsnotify.Watcher
	dirs     []string
	onChange func()
	debounce time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
}

// New creates a Watcher that monitors the given directories.
// The onChange callback is invoked (debounced) when any .json file in
// those directories is created, modified, or removed.
func New(dirs []string, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fsw:      fsw,
		dirs:     dirs,
		onChange: onChange,
		debounce: 500 * time.Millisecond,
		done:     make(chan struct{}),
	}

	for _, dir := range dirs {
		if err := fsw.Add(dir); err != nil {
			slog.Warn("configwatcher: unable to watch directory", "dir", dir, "err", err)
			// Not fatal — directory might not exist yet
		} else {
			slog.Info("configwatcher: watching", "dir", dir)
		}
	}

	return w, nil
}

// Start begins watching for changes in a background goroutine.
func (w *Watcher) Start() {
	w.wg.Add(1)
	go w.loop()
}

// Stop shuts down the watcher and waits for the background goroutine to exit.
func (w *Watcher) Stop() {
	close(w.done)
	w.fsw.Close()
	w.wg.Wait()
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	var timer *time.Timer

	for {
		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !isSettingsFile(event.Name) {
				continue
			}
			isWrite := event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
			if !isWrite {
				continue
			}

			slog.Info("configwatcher: settings changed", "file", filepath.Base(event.Name), "op", event.Op.String())

			// Debounce: editors often write a temp file then rename,
			// producing multiple events in quick succession.
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounce, w.onChange)

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Error("configwatcher: watch error", "err", err)
		}
	}
}

func isSettingsFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".json")
}
