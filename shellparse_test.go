package main

import (
	"reflect"
	"testing"
)

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
			"pipe splits into both sides",
			"curl https://example.com | jq .",
			[]string{"curl https://example.com", "jq ."},
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
