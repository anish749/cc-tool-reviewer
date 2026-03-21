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

type testCase struct {
	name         string
	toolName     string
	toolInput    any
	wantLocal    bool   // true = expect local match (empty response, no API call)
	wantAPI      bool   // true = must hit the API (non-empty response required)
	wantDecision string // expected decision: "allow" or "ask"
}

func runTestCases(t *testing.T, socketPath string, tests []testCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, elapsed := sendRequest(t, socketPath, tc.toolName, tc.toolInput)
			gotLocal := resp == ""

			if tc.wantLocal {
				if !gotLocal {
					t.Errorf("expected local match (empty response), got: %s", resp)
				}
				t.Logf("%-55s  %8s  local", tc.name, elapsed.Round(time.Millisecond))
				return
			}

			if tc.wantAPI {
				if gotLocal {
					t.Fatalf("expected API call (non-empty response), but got local match (empty response) — matcher incorrectly short-circuited")
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
					t.Errorf("decision: got %q, want %q (reason: %s)",
						got, tc.wantDecision, output.HookSpecificOutput.PermissionDecisionReason)
				}

				t.Logf("%-55s  %8s  api  decision=%s  reason=%s",
					tc.name, elapsed.Round(time.Millisecond),
					got, output.HookSpecificOutput.PermissionDecisionReason)
				return
			}

			t.Fatalf("test case must set either wantLocal or wantAPI")
		})
	}
}

func TestIntegration(t *testing.T) {
	socketPath := startTestServer(t)

	tests := []testCase{
		// --- Local matches: allow-listed ---
		{
			name:      "local/allow: rg",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "rg foo bar"},
			wantLocal: true,
		},
		{
			name:      "local/allow: go test",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "go test ./..."},
			wantLocal: true,
		},
		{
			name:      "local/allow: git status",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git status -s"},
			wantLocal: true,
		},
		{
			name:      "local/allow: npm install",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "npm install express"},
			wantLocal: true,
		},
		{
			name:      "local/allow: find",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "find . -name '*.go'"},
			wantLocal: true,
		},
		{
			name:      "local/allow: gh",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "gh pr list"},
			wantLocal: true,
		},
		// --- Local matches: deny-listed ---
		{
			name:      "local/deny: git reset --hard",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git reset --hard HEAD"},
			wantLocal: true,
		},
		{
			name:      "local/deny: git add -A",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git add -A"},
			wantLocal: true,
		},
		{
			name:      "local/deny: git branch -D",
			toolName:  "Bash",
			toolInput: map[string]string{"command": "git branch -D feature"},
			wantLocal: true,
		},
		// --- API: simple commands not in allow list ---
		{
			name:         "api/ask: docker build",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "docker build -t myapp ."},
			wantAPI:      true,
			wantDecision: "ask",
		},
		{
			name:         "api/ask: terraform plan",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "terraform plan"},
			wantAPI:      true,
			wantDecision: "ask",
		},
		{
			name:         "api/ask: kubectl apply",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "kubectl apply -f deployment.yaml"},
			wantAPI:      true,
			wantDecision: "ask",
		},
	}

	runTestCases(t, socketPath, tests)

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

func TestIntegrationComplex(t *testing.T) {
	socketPath := startTestServer(t)

	complexBashScript := `cd /Users/anish/git/cc-tool-reviewer && ./cc-tool-reviewer &
DAEMON_PID=$!
sleep 0.5

# Test 1: allow-listed (rg)
echo 'Test 1: allow-listed command (rg foo bar)'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Bash","tool_input":{"command":"rg foo bar"}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '${RESULT:-(empty)}'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"
echo ""

# Test 2: deny-listed (git reset --hard)
echo 'Test 2: deny-listed command (git reset --hard HEAD)'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Bash","tool_input":{"command":"git reset --hard HEAD"}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '${RESULT:-(empty)}'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"
echo ""

# Test 3: ask-zone, cold connection
echo 'Test 3: ask-zone (docker build) — cold API connection'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Bash","tool_input":{"command":"docker build -t myapp ."}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '$RESULT'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"
echo ""

# Test 4: ask-zone, warm connection
echo 'Test 4: ask-zone (terraform apply) — warm API connection'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Bash","tool_input":{"command":"terraform apply -auto-approve"}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '$RESULT'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"
echo ""

# Test 5: piped allowed commands
echo 'Test 5: piped allowed commands (rg | grep | sort)'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Bash","tool_input":{"command":"rg TODO src/ | grep -v node_modules | sort"}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '${RESULT:-(empty)}'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"
echo ""

# Test 6: Edit tool (ask-zone)
echo 'Test 6: Edit tool — warm API connection'
START=$(python3 -c 'import time; print(time.time())')
RESULT=$(echo '{"tool_name":"Edit","tool_input":{"file_path":"/tmp/test.go","old_string":"foo","new_string":"bar"}}' | nc -U /tmp/cc-tool-reviewer.sock)
END=$(python3 -c 'import time; print(time.time())')
echo "Result: '$RESULT'"
python3 -c "print(f'Time: {($END - $START)*1000:.1f}ms')"

kill $DAEMON_PID 2>/dev/null
wait $DAEMON_PID 2>/dev/null`

	tests := []testCase{
		// Complex multi-line bash script composing many allowed commands.
		// Must hit API (not be short-circuited by local matcher).
		// AI should recognize all executed commands are individually allowed.
		{
			name:         "api/allow: complex bash script composing allowed commands",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": complexBashScript},
			wantAPI:      true,
			wantDecision: "allow",
		},
		// cd && git log — compound with &&, both parts individually allowed.
		// Must hit API (compound commands bypass local matcher).
		{
			name:         "api/allow: cd && git log (compound allowed)",
			toolName:     "Bash",
			toolInput:    map[string]string{"command": "cd ~/git/x && git log"},
			wantAPI:      true,
			wantDecision: "allow",
		},
	}

	runTestCases(t, socketPath, tests)
}
