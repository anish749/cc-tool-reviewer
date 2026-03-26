package main

import (
	"encoding/json"
	"testing"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		raw      string
		wantOK   bool
		wantTool string
		wantPat  string
	}{
		{"Bash(curl:*)", true, "Bash", "curl:*"},
		{"Bash(rg:*)", true, "Bash", "rg:*"},
		{"Bash(git status *)", true, "Bash", "git status *"},
		{"WebSearch", true, "WebSearch", "*"},
		{"", false, "", ""},
		{"Bash()", false, "", ""}, // empty inner pattern won't match regex (.+)
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			r, ok := ParseRule(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ParseRule(%q) ok = %v, want %v", tc.raw, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if r.Tool != tc.wantTool || r.Pattern != tc.wantPat {
				t.Errorf("ParseRule(%q) = {%q, %q}, want {%q, %q}",
					tc.raw, r.Tool, r.Pattern, tc.wantTool, tc.wantPat)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		want    bool
	}{
		{"wildcard", "anything", "*", true},
		{"colon-star exact", "curl", "curl:*", true},
		{"colon-star prefix", "curl https://example.com", "curl:*", true},
		{"colon-star no match", "wget https://example.com", "curl:*", false},
		{"space-star exact", "git status", "git status *", true},
		{"space-star prefix", "git status -s", "git status *", true},
		{"space-star no match", "git log", "git status *", false},
		{"suffix-star", "git branch -D feature", "git branch -D*", true},
		{"exact match", "python3", "python3", true},
		{"exact no match", "python2", "python3", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchPattern(tc.input, tc.pattern)
			if got != tc.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v",
					tc.input, tc.pattern, got, tc.want)
			}
		})
	}
}

func bashInput(command string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"command": command})
	return b
}

// --- Simple (non-compound) commands ---

func TestMatchesAll_CurlSimple(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	got := MatchesAll("Bash", bashInput("curl https://example.com"), rules)
	if !got {
		t.Error("simple curl should match Bash(curl:*)")
	}
}

func TestMatchesAll_CurlWithPipe(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "jq:*"},
	}

	cmd := "curl -s https://example.com | jq ."
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("curl|jq with both allowed should match")
	}
}

func TestMatchesAll_CurlWithPipePartial(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	// jq is not in allow list → should NOT match
	cmd := "curl -s https://example.com | jq ."
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("curl|jq with only curl allowed should NOT match")
	}
}

func TestMatchesAny_DenyPipe(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "rm:*"}}

	// rm on the right side of a pipe should be caught by deny
	cmd := "cat /etc/passwd | rm -rf /"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("denied command in pipe should be caught")
	}
}

func TestMatchesAll_CurlMultilineJSON(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "python3:*"},
	}

	cmd := `curl -s 'http://video-elasticsearch-client.service.tubular:9200/intelligence/_search' -H 'Content-Type: application/json' -d '{
        "size": 0,
        "aggs": {
          "missing_is_public": {
            "missing": {
              "field": "is_public"
            }
          }
        }
      }' | python3 -m json.tool`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("multiline curl piped to python3 with both allowed should match")
	}
}

func TestMatchesAll_CurlSingleLineJSON(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "python3:*"},
	}

	cmd := `curl -s 'http://example.com' -d '{"size":0}' | python3 -m json.tool`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("single-line curl piped to python3 should match")
	}
}

// --- Compound commands: MatchesAll (allow rules) ---

func TestMatchesAll_AllMatch(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := "git add . && git commit -m 'fix' && echo done"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("all sub-commands match → should match")
	}
}

func TestMatchesAll_PartialMatch(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := "curl https://example.com && rm -rf /tmp/data"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("partial match (rm unmatched) → should NOT match")
	}
}

func TestMatchesAll_NoneMatch(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := "wget https://example.com && rm -rf /tmp/data"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("no sub-commands match → should NOT match")
	}
}

func TestMatchesAll_Semicolon(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "git:*"}}

	cmd := "git add .; git commit -m 'msg'"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("semicolon-separated, all matching → should match")
	}
}

func TestMatchesAll_Newline(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := "git status\necho 'all clean'"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("newline-separated, all matching → should match")
	}
}

// --- Compound commands: MatchesAny (deny rules) ---

func TestMatchesAny_DenyAnyMatch(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "git reset *"}}

	cmd := "git reset --hard HEAD && git push"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("one sub-command matches deny rule → should match")
	}
}

func TestMatchesAny_DenyNoneMatch(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "git reset *"}}

	cmd := "git add . && git commit -m 'msg'"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if got {
		t.Error("no sub-commands match deny rules → should NOT match")
	}
}

func TestMatchesAny_DenyAllMatch(t *testing.T) {
	denyRules := []Rule{
		{Tool: "Bash", Pattern: "git reset *"},
		{Tool: "Bash", Pattern: "git push *"},
	}

	cmd := "git reset --hard HEAD && git push --force"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("all sub-commands match deny rules → should match")
	}
}

// --- Subshell handling ---

func TestMatchesAll_SubshellAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "whoami:*"},
	}

	cmd := "echo $(whoami)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("subshell command also allowed → should match")
	}
}

func TestMatchesAll_SubshellNotAllowed(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	cmd := "echo $(whoami)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("subshell command not allowed → should NOT match")
	}
}

func TestMatchesAll_SubshellDangerous(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	cmd := "echo $(rm -rf /)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("dangerous subshell → should NOT match")
	}
}

func TestMatchesAll_NestedSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := "echo $(echo $(date))"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("all nested subshell commands allowed → should match")
	}
}

func TestMatchesAll_NestedSubshellPartial(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	cmd := "echo $(echo $(date))"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("inner nested subshell not allowed → should NOT match")
	}
}

func TestMatchesAll_BacktickSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := "echo `date`"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("backtick subshell allowed → should match")
	}
}

func TestMatchesAll_BacktickSubshellNotAllowed(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	cmd := "echo `whoami`"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("backtick subshell not allowed → should NOT match")
	}
}

func TestMatchesAll_SubshellInDoubleQuotes(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := `echo "today is $(date)"`
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("subshell in double quotes, all allowed → should match")
	}
}

func TestMatchesAll_SubshellInSingleQuotes(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	// $() inside single quotes is literal — not a subshell
	cmd := "echo '$(date)'"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("subshell in single quotes is literal, not compound → should match")
	}
}

func TestMatchesAll_CompoundWithSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := "git add . && echo $(date)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("compound + subshell, all allowed → should match")
	}
}

func TestMatchesAll_CompoundWithSubshellPartial(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	// git and echo allowed, but date (inside subshell) is not
	cmd := "git add . && echo $(date)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("subshell content not allowed → should NOT match")
	}
}

func TestMatchesAny_DenySubshell(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "rm:*"}}

	cmd := "echo $(rm -rf /tmp/data)"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("denied command inside subshell → should match")
	}
}

func TestMatchesAll_SubshellWithCompoundInside(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := "echo $(git status && date)"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("compound inside subshell, all allowed → should match")
	}
}

func TestMatchesAll_EmptyCommand(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "git:*"}}

	got := MatchesAll("Bash", bashInput(""), rules)
	if got {
		t.Error("empty command should NOT match any allow list")
	}
}
