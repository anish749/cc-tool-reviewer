package main

import (
	"reflect"
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

		// Operators inside double quotes — subshells expand, separators don't
		{"semicolon in double quotes", `echo "a;b"`, false},
		{"double-amp in double quotes", `echo "a && b"`, false},
		{"newline in double quotes", "echo \"line1\nline2\"", false},
		{"subshell in double quotes", `echo "$(date)"`, true},  // expands!
		{"backtick in double quotes", "echo \"`date`\"", true}, // expands!

		// Pipe with subshell on one side — IS compound
		{"pipe with subshell", "echo $(date) | jq .", true},

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
			"compound &&",
			"git add . && git commit -m 'msg'",
			[]string{"git add .", "git commit -m 'msg'"},
		},
		{
			"compound semicolon",
			"cd /tmp; ls -la",
			[]string{"cd /tmp", "ls -la"},
		},
		{
			"compound newline",
			"echo hello\necho world",
			[]string{"echo hello", "echo world"},
		},
		{
			"three chained git commands",
			"git add . && git commit -m 'fix' && git push",
			[]string{"git add .", "git commit -m 'fix'", "git push"},
		},
		{
			"pipe kept as one command",
			"curl https://example.com | jq .",
			[]string{"curl https://example.com | jq ."},
		},
		{
			"simple subshell",
			"echo $(whoami)",
			[]string{"echo $(whoami)", "whoami"},
		},
		{
			"nested subshell",
			"echo $(echo $(date))",
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
