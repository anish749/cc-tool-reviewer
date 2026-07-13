package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func clearBackendEnv(t *testing.T) {
	t.Helper()
	for _, name := range anthropicEnvVars {
		t.Setenv(name, "")
	}
}

func TestUseAnthropicSDK(t *testing.T) {
	for _, name := range anthropicEnvVars {
		t.Run(name, func(t *testing.T) {
			clearBackendEnv(t)
			if UseAnthropicSDK() {
				t.Fatal("expected false with no env set")
			}
			t.Setenv(name, "x")
			if !UseAnthropicSDK() {
				t.Errorf("expected true with %s set", name)
			}
		})
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

func TestValidate_DefaultsToCLI(t *testing.T) {
	clearBackendEnv(t)

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
