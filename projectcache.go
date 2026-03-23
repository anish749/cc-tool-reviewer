package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jellydator/ttlcache/v3"
)

// ProjectRules holds cached allow/deny rules for a project directory.
type ProjectRules struct {
	Allow    []Rule
	Deny     []Rule
	RawAllow []string
}

// ProjectCache caches project-level settings with TTL and fsnotify invalidation.
type ProjectCache struct {
	cache   *ttlcache.Cache[string, ProjectRules]
	watcher *fsnotify.Watcher
	watched map[string]bool // directories we're already watching
}

// NewProjectCache creates a cache with the given TTL.
func NewProjectCache(ttl time.Duration) (*ProjectCache, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	cache := ttlcache.New[string, ProjectRules](
		ttlcache.WithTTL[string, ProjectRules](ttl),
	)
	go cache.Start() // background cleanup of expired items

	pc := &ProjectCache{
		cache:   cache,
		watcher: fsw,
		watched: make(map[string]bool),
	}

	go pc.watchLoop()

	return pc, nil
}

// Get returns cached project rules for the given cwd, loading from disk on cache miss.
func (pc *ProjectCache) Get(cwd string) ProjectRules {
	if cwd == "" {
		return ProjectRules{}
	}

	item := pc.cache.Get(cwd)
	if item != nil {
		return item.Value()
	}

	// Cache miss — load from disk
	allow, deny, rawAllow := LoadProjectRules(cwd)
	rules := ProjectRules{Allow: allow, Deny: deny, RawAllow: rawAllow}

	pc.cache.Set(cwd, rules, ttlcache.DefaultTTL)

	// Start watching this project's .claude/ directory if we haven't already
	pc.watchDir(cwd)

	return rules
}

// Stop shuts down the cache and watcher.
func (pc *ProjectCache) Stop() {
	pc.cache.Stop()
	pc.watcher.Close()
}

func (pc *ProjectCache) watchDir(cwd string) {
	claudeDir := filepath.Join(cwd, ".claude")
	if pc.watched[claudeDir] {
		return
	}

	// Only watch if the directory exists
	if _, err := os.Stat(claudeDir); err != nil {
		return
	}

	if err := pc.watcher.Add(claudeDir); err != nil {
		slog.Warn("projectcache: unable to watch", "dir", claudeDir, "err", err)
		return
	}

	pc.watched[claudeDir] = true
	slog.Info("projectcache: watching", "dir", claudeDir)
}

func (pc *ProjectCache) watchLoop() {
	for {
		select {
		case event, ok := <-pc.watcher.Events:
			if !ok {
				return
			}
			if !isSettingsJSON(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			// The changed file is inside <cwd>/.claude/ — derive the cwd
			claudeDir := filepath.Dir(event.Name)
			cwd := filepath.Dir(claudeDir)

			slog.Info("projectcache: invalidating", "cwd", cwd, "file", filepath.Base(event.Name))
			pc.cache.Delete(cwd)

		case err, ok := <-pc.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("projectcache: watch error", "err", err)
		}
	}
}

func isSettingsJSON(path string) bool {
	name := filepath.Base(path)
	return name == "settings.json" || name == "settings.local.json"
}
