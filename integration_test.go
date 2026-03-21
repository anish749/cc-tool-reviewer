package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sendRequest(t *testing.T, socketPath string, toolName string, toolInput any) (string, time.Duration) {
	t.Helper()

	inputJSON, err := json.Marshal(toolInput)
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}

	hookInput := HookInput{
		ToolName:  toolName,
		ToolInput: json.RawMessage(inputJSON),
	}
	reqBody, err := json.Marshal(hookInput)
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}

	start := time.Now()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(30 * time.Second))

	if _, err := conn.Write(reqBody); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Signal that we're done writing so the server's io.ReadAll returns.
	conn.(*net.UnixConn).CloseWrite()

	resp, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	elapsed := time.Since(start)
	return string(resp), elapsed
}

func startTestServer(t *testing.T) string {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "test.sock")

	allow, deny, rawAllow := LoadRules()
	reviewer := NewReviewer(rawAllow)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := NewServer(listener, allow, deny, reviewer)
	go server.Serve()

	t.Cleanup(func() {
		listener.Close()
		os.Remove(socketPath)
	})

	return socketPath
}

func TestIntegration(t *testing.T) {
	socketPath := startTestServer(t)

	type testCase struct {
		name           string
		toolName       string
		toolInput      any
		wantEmpty      bool   // true = expect local match (empty response)
		wantDecision   string // if not empty, expect this decision from AI
	}

	tests := []testCase{
		{
			name:      "allow-listed: rg",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "rg foo bar"},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: go test",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "go test ./..."},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: git status",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git status -s"},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: npm install",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "npm install express"},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: find",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "find . -name '*.go'"},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: gh",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "gh pr list"},
			wantEmpty: true,
		},
		{
			name:      "allow-listed: piped starts with allowed (find | xargs grep)",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "find . -name '*.go' | xargs grep TODO"},
			wantEmpty: true,
		},
		{
			name:      "deny-listed: git reset --hard",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git reset --hard HEAD"},
			wantEmpty: true,
		},
		{
			name:      "deny-listed: git add -A",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git add -A"},
			wantEmpty: true,
		},
		{
			name:      "deny-listed: git branch -D",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git branch -D feature"},
			wantEmpty: true,
		},
		{
			name:         "ask-zone: docker build",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "docker build -t myapp ."},
			wantDecision: "ask",
		},
		{
			name:         "ask-zone: terraform plan",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "terraform plan"},
			wantDecision: "ask",
		},
		{
			name:         "ask-zone: kubectl apply",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "kubectl apply -f deployment.yaml"},
			wantDecision: "ask",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, elapsed := sendRequest(t, socketPath, tc.toolName, tc.toolInput)

			if tc.wantEmpty {
				if resp != "" {
					t.Errorf("expected empty response, got: %s", resp)
				}
				t.Logf("%-55s  %8s  (local match)", tc.name, elapsed.Round(time.Millisecond))
				return
			}

			if resp == "" {
				t.Fatalf("expected non-empty response with decision=%q, got empty", tc.wantDecision)
			}

			var output HookOutput
			if err := json.Unmarshal([]byte(resp), &output); err != nil {
				t.Fatalf("unmarshal response: %v (raw: %s)", err, resp)
			}

			if output.HookSpecificOutput == nil {
				t.Fatalf("expected hookSpecificOutput, got nil")
			}

			got := output.HookSpecificOutput.PermissionDecision
			if tc.wantDecision != "" && got != tc.wantDecision {
				t.Errorf("decision: got %q, want %q (reason: %s)", got, tc.wantDecision, output.HookSpecificOutput.PermissionDecisionReason)
			}

			t.Logf("%-55s  %8s  decision=%s  reason=%s",
				tc.name, elapsed.Round(time.Millisecond),
				got, output.HookSpecificOutput.PermissionDecisionReason)
		})
	}

	// Print summary
	fmt.Println("\n--- Timing Summary ---")
	for _, tc := range tests {
		resp, elapsed := sendRequest(t, socketPath, tc.toolName, tc.toolInput)
		kind := "local"
		decision := "-"
		if resp != "" {
			kind = "api"
			var output HookOutput
			json.Unmarshal([]byte(resp), &output)
			if output.HookSpecificOutput != nil {
				decision = output.HookSpecificOutput.PermissionDecision
			}
		}
		fmt.Printf("  %-55s  %8s  %-5s  decision=%s\n", tc.name, elapsed.Round(time.Millisecond), kind, decision)
	}
}
