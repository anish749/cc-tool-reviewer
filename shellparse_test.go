package main

import (
	"reflect"
	"testing"
)

func TestCollectAllCommands_CommentThenCurl(t *testing.T) {
	cmd := "# fetch recent items\ncurl -s 'http://example.com' -d '{\"limit\":5}'"
	got := CollectAllCommands(cmd)
	// Shell comments are not commands — parser should only return curl
	want := []string{"curl -s 'http://example.com' -d '{\"limit\":5}'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectAllCommands(%q)\n  got  %v\n  want %v", cmd, got, want)
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
		// --- Control-flow constructs ---
		{
			"for loop",
			"for f in *.go; do echo $f; done",
			[]string{"echo $f"},
		},
		{
			"for loop with subshell",
			"for svc in a b; do result=$(curl -s http://example.com); echo $result; done",
			[]string{"curl -s http://example.com", "echo $result"},
		},
		{
			"for loop with if inside",
			`for svc in a b; do
        result=$(curl -s "http://example.com" 2>/dev/null)
        count=$(echo "$result" | python3 -c "import json,sys; print('ok')" 2>/dev/null)
        if [ "$count" != "0" ]; then
          echo "$svc: $count hits"
        fi
      done`,
			[]string{
				`curl -s "http://example.com"`,
				`echo "$result"`,
				`python3 -c "import json,sys; print('ok')"`,
				`[ "$count" != "0" ]`,
				`echo "$svc: $count hits"`,
			},
		},
		{
			"if-else",
			`if [ -f go.mod ]; then echo found; else echo missing; fi`,
			[]string{"[ -f go.mod ]", "echo found", "echo missing"},
		},
		{
			"if-elif-else",
			`if [ "$x" = "a" ]; then echo a; elif [ "$x" = "b" ]; then echo b; else echo other; fi`,
			[]string{`[ "$x" = "a" ]`, "echo a", `[ "$x" = "b" ]`, "echo b", "echo other"},
		},
		{
			"while loop",
			"while true; do echo waiting; sleep 1; done",
			[]string{"true", "echo waiting", "sleep 1"},
		},
		{
			"case statement",
			`case "$1" in start) echo starting;; stop) echo stopping;; esac`,
			[]string{"echo starting", "echo stopping"},
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
