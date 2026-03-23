package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Permissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type Settings struct {
	Permissions Permissions `json:"permissions"`
}

func claudeConfigDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// LoadRules loads global allow/deny rules from the Claude config directory.
func LoadRules() (allow, deny []Rule, rawAllow []string) {
	configDir := claudeConfigDir()

	paths := []string{
		filepath.Join(configDir, "settings.json"),
		filepath.Join(configDir, "settings.local.json"),
	}

	return loadFromPaths(paths)
}

// LoadProjectRules loads project-level allow/deny rules from the given working directory.
func LoadProjectRules(cwd string) (allow, deny []Rule, rawAllow []string) {
	if cwd == "" {
		return
	}

	paths := []string{
		filepath.Join(cwd, ".claude", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.local.json"),
	}

	return loadFromPaths(paths)
}

func loadFromPaths(paths []string) (allow, deny []Rule, rawAllow []string) {
	seen := make(map[string]bool)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s Settings
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		for _, raw := range s.Permissions.Allow {
			if r, ok := ParseRule(raw); ok {
				allow = append(allow, r)
			}
			if !seen[raw] {
				rawAllow = append(rawAllow, raw)
				seen[raw] = true
			}
		}
		for _, raw := range s.Permissions.Deny {
			if r, ok := ParseRule(raw); ok {
				deny = append(deny, r)
			}
		}
	}
	return
}
