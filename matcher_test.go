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

func TestMatchesAny_CurlSimple(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	got := MatchesAny("Bash", bashInput("curl https://example.com"), rules, MatchAll)
	if !got {
		t.Error("simple curl should match Bash(curl:*)")
	}
}

func TestMatchesAny_CurlWithPipe(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := "curl -s https://example.com | jq ."
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("curl with pipe should match Bash(curl:*)")
	}
}

func TestMatchesAny_CurlMultilineJSON(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

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

	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("multiline curl with JSON body should match Bash(curl:*)")
	}
}

func TestMatchesAny_CurlSingleLineJSON(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := `curl -s 'http://example.com' -d '{"size":0}' | python3 -m json.tool`

	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("single-line curl should match Bash(curl:*)")
	}
}

// --- Compound commands: MatchAll mode (allow rules) ---

func TestMatchesAny_AllowAllMatch(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := "git add . && git commit -m 'fix' && echo done"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: all sub-commands match → should match")
	}
}

func TestMatchesAny_AllowPartialMatch(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := "curl https://example.com && rm -rf /tmp/data"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: partial match (rm unmatched) → should NOT match")
	}
}

func TestMatchesAny_AllowNoneMatch(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "curl:*"}}

	cmd := "wget https://example.com && rm -rf /tmp/data"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: no sub-commands match → should NOT match")
	}
}

func TestMatchesAny_AllowSemicolon(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "git:*"}}

	cmd := "git add .; git commit -m 'msg'"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: semicolon-separated, all matching → should match")
	}
}

func TestMatchesAny_AllowNewline(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := "git status\necho 'all clean'"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: newline-separated, all matching → should match")
	}
}

// --- Compound commands: MatchAny mode (deny rules) ---

func TestMatchesAny_DenyAnyMatch(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "git reset *"}}

	// Only first sub-command matches the deny rule — should still deny locally
	cmd := "git reset --hard HEAD && git push"
	got := MatchesAny("Bash", bashInput(cmd), denyRules, MatchAny)
	if !got {
		t.Error("MatchAny: one sub-command matches deny rule → should match")
	}
}

func TestMatchesAny_DenyNoneMatch(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "git reset *"}}

	cmd := "git add . && git commit -m 'msg'"
	got := MatchesAny("Bash", bashInput(cmd), denyRules, MatchAny)
	if got {
		t.Error("MatchAny: no sub-commands match deny rules → should NOT match")
	}
}

func TestMatchesAny_DenyAllMatch(t *testing.T) {
	denyRules := []Rule{
		{Tool: "Bash", Pattern: "git reset *"},
		{Tool: "Bash", Pattern: "git push *"},
	}

	cmd := "git reset --hard HEAD && git push --force"
	got := MatchesAny("Bash", bashInput(cmd), denyRules, MatchAny)
	if !got {
		t.Error("MatchAny: all sub-commands match deny rules → should match")
	}
}

// --- Subshell handling ---

func TestMatchesAny_SubshellAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "whoami:*"},
	}

	// Both echo and whoami are allowed → local match
	cmd := "echo $(whoami)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: subshell command also allowed → should match")
	}
}

func TestMatchesAny_SubshellNotAllowed(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	// echo is allowed but whoami is not → send to AI
	cmd := "echo $(whoami)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: subshell command not allowed → should NOT match")
	}
}

func TestMatchesAny_SubshellDangerous(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	// echo is allowed but rm is not → send to AI
	cmd := "echo $(rm -rf /)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: dangerous subshell → should NOT match")
	}
}

func TestMatchesAny_NestedSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	// All levels allowed → local match
	cmd := "echo $(echo $(date))"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: all nested subshell commands allowed → should match")
	}
}

func TestMatchesAny_NestedSubshellPartial(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
	}

	// echo is allowed but date is not → send to AI
	cmd := "echo $(echo $(date))"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: inner nested subshell not allowed → should NOT match")
	}
}

func TestMatchesAny_BacktickSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	cmd := "echo `date`"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: backtick subshell allowed → should match")
	}
}

func TestMatchesAny_BacktickSubshellNotAllowed(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	cmd := "echo `whoami`"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: backtick subshell not allowed → should NOT match")
	}
}

func TestMatchesAny_SubshellInDoubleQuotes(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	// $() inside double quotes still expands
	cmd := `echo "today is $(date)"`
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: subshell in double quotes, all allowed → should match")
	}
}

func TestMatchesAny_SubshellInSingleQuotes(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "echo:*"}}

	// $() inside single quotes is literal — not a subshell
	cmd := "echo '$(date)'"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("subshell in single quotes is literal, not compound → should match")
	}
}

func TestMatchesAny_CompoundWithSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	// Compound + subshell, all parts allowed
	cmd := "git add . && echo $(date)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: compound + subshell, all allowed → should match")
	}
}

func TestMatchesAny_CompoundWithSubshellPartial(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	// git and echo allowed, but date (inside subshell) is not
	cmd := "git add . && echo $(date)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if got {
		t.Error("MatchAll: subshell content not allowed → should NOT match")
	}
}

func TestMatchesAny_DenySubshell(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "rm:*"}}

	// deny mode: rm inside subshell matches → deny locally
	cmd := "echo $(rm -rf /tmp/data)"
	got := MatchesAny("Bash", bashInput(cmd), denyRules, MatchAny)
	if !got {
		t.Error("MatchAny: denied command inside subshell → should match")
	}
}

func TestMatchesAny_SubshellWithCompoundInside(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "git:*"},
		{Tool: "Bash", Pattern: "date:*"},
	}

	// Compound command inside subshell
	cmd := "echo $(git status && date)"
	got := MatchesAny("Bash", bashInput(cmd), rules, MatchAll)
	if !got {
		t.Error("MatchAll: compound inside subshell, all allowed → should match")
	}
}
