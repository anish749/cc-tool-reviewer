package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearBackendEnv(t *testing.T) {
	t.Helper()
	for _, name := range append([]string{EnvUseCLIClient}, anthropicEnvVars...) {
		t.Setenv(name, "")
	}
}

func TestValidate_NothingSet(t *testing.T) {
	clearBackendEnv(t)
	err := Validate()
	if err == nil {
		t.Fatal("expected error with no backend configured")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("error should name the fix: %v", err)
	}
}

func TestValidate_AnthropicVars(t *testing.T) {
	for _, name := range anthropicEnvVars {
		t.Run(name, func(t *testing.T) {
			clearBackendEnv(t)
			t.Setenv(name, "x")
			if err := Validate(); err != nil {
				t.Errorf("Validate with %s set = %v, want nil", name, err)
			}
		})
	}
}

func TestValidate_CLIMode(t *testing.T) {
	clearBackendEnv(t)
	t.Setenv(EnvUseCLIClient, "1")

	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := Validate(); err == nil {
		t.Error("expected error when claude binary is not on PATH")
	}

	fake := filepath.Join(dir, "claude")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	if err := Validate(); err != nil {
		t.Errorf("Validate with claude on PATH = %v, want nil", err)
	}
}
