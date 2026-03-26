package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestIsCompoundCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		// Simple commands — not compound
		{"simple command", "curl https://example.com", false},
		{"command with flags", "git commit -m 'hello world'", false},
		{"pipe chain", "curl https://example.com | jq .", false},
		{"multi-pipe", "cat file | grep foo | sort | uniq", false},

		// Separators outside quotes — compound
		{"semicolon", "echo foo; echo bar", true},
		{"double-amp", "cd /tmp && ls", true},
		{"double-pipe", "cmd1 || cmd2", true},
		{"newline", "echo foo\necho bar", true},

		// Subshells — compound
		{"dollar-paren", "echo $(whoami)", true},
		{"backtick", "echo `whoami`", true},

		// Operators inside single quotes — NOT compound (literal)
		{"semicolon in single quotes", "echo 'a;b'", false},
		{"double-amp in single quotes", "echo 'a && b'", false},
		{"newline in single quotes", "curl -d '{\n\"key\": 1}'", false},
		{"subshell in single quotes", "echo '$(date)'", false},
		{"backtick in single quotes", "echo '`date`'", false},
		{"double-pipe in single quotes", "echo 'a || b'", false},

		// Operators inside double quotes — subshells expand, separators don't
		{"semicolon in double quotes", `echo "a;b"`, false},
		{"double-amp in double quotes", `echo "a && b"`, false},
		{"newline in double quotes", "echo \"line1\nline2\"", false},
		{"subshell in double quotes", `echo "$(date)"`, true},  // expands!
		{"backtick in double quotes", "echo \"`date`\"", true}, // expands!

		// Mixed quoting
		{"escaped backslash before quote", `echo \\; ls`, true}, // \\ is literal backslash, ; is real
		{"single amp (background) not flagged", "sleep 10 &", false},

		// Real-world: multiline curl with JSON body
		{
			"multiline curl with JSON in single quotes",
			"curl -s 'http://example.com/search' -H 'Content-Type: application/json' -d '{\n        \"size\": 0,\n        \"aggs\": {}\n      }'",
			false,
		},
		{
			"multiline curl piped to python",
			"curl -s 'http://example.com' -d '{\n  \"q\": 1\n}' | python3 -m json.tool",
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isCompoundCommand(tc.cmd)
			if got != tc.want {
				t.Errorf("isCompoundCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestSplitCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
	}{
		// No splitting needed
		{
			"simple command",
			"curl https://example.com",
			[]string{"curl https://example.com"},
		},
		{
			"pipe chain (not split)",
			"curl https://example.com | jq .",
			[]string{"curl https://example.com | jq ."},
		},

		// Split on &&
		{
			"double-amp",
			"git add . && git commit -m 'msg'",
			[]string{"git add .", "git commit -m 'msg'"},
		},

		// Split on ||
		{
			"double-pipe",
			"make build || echo 'build failed'",
			[]string{"make build", "echo 'build failed'"},
		},

		// Split on semicolon
		{
			"semicolon",
			"cd /tmp; ls -la",
			[]string{"cd /tmp", "ls -la"},
		},

		// Split on newline
		{
			"newline",
			"echo hello\necho world",
			[]string{"echo hello", "echo world"},
		},

		// Multiple operators
		{
			"mixed operators",
			"git add . && git commit -m 'msg'; git push",
			[]string{"git add .", "git commit -m 'msg'", "git push"},
		},

		// Operators inside quotes are NOT separators
		{
			"&& inside single quotes",
			"echo 'a && b' && echo done",
			[]string{"echo 'a && b'", "echo done"},
		},
		{
			"semicolon inside double quotes",
			`echo "a;b"; echo done`,
			[]string{`echo "a;b"`, "echo done"},
		},
		{
			"newline inside single quotes",
			"curl -d '{\n  \"key\": 1\n}' && echo ok",
			[]string{"curl -d '{\n  \"key\": 1\n}'", "echo ok"},
		},

		// Empty parts are omitted
		{
			"trailing semicolon",
			"echo hello;",
			[]string{"echo hello"},
		},
		{
			"multiple semicolons",
			";;echo hello;;",
			[]string{"echo hello"},
		},
		{
			"empty newlines",
			"\n\necho hello\n\n",
			[]string{"echo hello"},
		},

		// Whitespace trimming
		{
			"whitespace around operators",
			"  git add .  &&  git commit -m 'msg'  ",
			[]string{"git add .", "git commit -m 'msg'"},
		},

		// Pipe within a split — pipes stay with their command
		{
			"pipe within &&-separated commands",
			"curl https://example.com | jq . && echo done",
			[]string{"curl https://example.com | jq .", "echo done"},
		},

		// Escaped characters
		{
			"escaped semicolon (not a separator)",
			`echo hello\; world`,
			[]string{`echo hello\; world`},
		},

		// Complex real-world
		{
			"cd && git log",
			"cd ~/git/x && git log --oneline -5",
			[]string{"cd ~/git/x", "git log --oneline -5"},
		},
		{
			"three chained git commands",
			"git add . && git commit -m 'fix' && git push",
			[]string{"git add .", "git commit -m 'fix'", "git push"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitCommands(tc.cmd)
			if len(got) != len(tc.want) {
				t.Fatalf("SplitCommands(%q) = %v (len %d), want %v (len %d)",
					tc.cmd, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("SplitCommands(%q)[%d] = %q, want %q",
						tc.cmd, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSplitCommandsQuotePreservation(t *testing.T) {
	cmd := `git commit -m "fix: handle edge case" && git push origin main`
	got := SplitCommands(cmd)
	want := []string{
		`git commit -m "fix: handle edge case"`,
		"git push origin main",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitCommandsLargeCompound(t *testing.T) {
	script := strings.Join([]string{
		"cd /tmp && ./server &",
		"PID=$!",
		"sleep 0.5",
		"",
		"echo 'Test 1'",
		"curl http://localhost:8080",
		"",
		"kill $PID 2>/dev/null",
	}, "\n")

	got := SplitCommands(script)
	if len(got) < 5 {
		t.Errorf("expected at least 5 commands from script, got %d: %v", len(got), got)
	}
}

// --- extractSubshells ---

func TestExtractSubshells(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
	}{
		{"no subshells", "echo hello", nil},
		{"simple $()", "echo $(whoami)", []string{"whoami"}},
		{"simple backtick", "echo `date`", []string{"date"}},
		{"nested $()", "echo $(echo $(date))", []string{"echo $(date)"}},
		{"multiple $()", "echo $(whoami) $(date)", []string{"whoami", "date"}},
		{"$() in single quotes (suppressed)", "echo '$(whoami)'", nil},
		{"backtick in single quotes (suppressed)", "echo '`date`'", nil},
		{"$() in double quotes (expands)", `echo "$(whoami)"`, []string{"whoami"}},
		{"backtick in double quotes (expands)", "echo \"`date`\"", []string{"date"}},
		{
			"compound inside $()",
			"echo $(git status && date)",
			[]string{"git status && date"},
		},
		{
			"$() with quotes inside",
			`echo $(grep "hello world" file.txt)`,
			[]string{`grep "hello world" file.txt`},
		},
		{
			"$() with nested parens",
			"echo $(echo $((1+2)))",
			[]string{"echo $((1+2))"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSubshells(tc.cmd)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractSubshells(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

// --- CollectAllCommands ---

func TestCollectAllCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
	}{
		{
			"simple command",
			"echo hello",
			[]string{"echo hello"},
		},
		{
			"compound without subshells",
			"git add . && git commit -m 'msg'",
			[]string{"git add .", "git commit -m 'msg'"},
		},
		{
			"simple subshell",
			"echo $(whoami)",
			// outer command + inner subshell command
			[]string{"echo $(whoami)", "whoami"},
		},
		{
			"nested subshell",
			"echo $(echo $(date))",
			// Level 0: "echo $(echo $(date))"
			// Level 1: "echo $(date)"
			// Level 2: "date"
			[]string{"echo $(echo $(date))", "echo $(date)", "date"},
		},
		{
			"compound + subshell",
			"git add . && echo $(date)",
			[]string{"git add .", "echo $(date)", "date"},
		},
		{
			"compound inside subshell",
			"echo $(git status && date)",
			// outer: "echo $(git status && date)"
			// subshell content "git status && date" is split:
			[]string{"echo $(git status && date)", "git status", "date"},
		},
		{
			"backtick subshell",
			"echo `date`",
			[]string{"echo `date`", "date"},
		},
		{
			"subshell in single quotes (not collected)",
			"echo '$(date)'",
			[]string{"echo '$(date)'"},
		},
		{
			"multiple subshells",
			"echo $(whoami) $(date)",
			[]string{"echo $(whoami) $(date)", "whoami", "date"},
		},
		{
			"deeply nested compound",
			"echo $(git add . && echo $(date))",
			// outer: "echo $(git add . && echo $(date))"
			// subshell: "git add . && echo $(date)" → split → "git add .", "echo $(date)"
			// "echo $(date)" has subshell → "date"
			[]string{
				"echo $(git add . && echo $(date))",
				"git add .",
				"echo $(date)",
				"date",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CollectAllCommands(tc.cmd)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CollectAllCommands(%q)\n  got  %v\n  want %v", tc.cmd, got, tc.want)
			}
		})
	}
}
