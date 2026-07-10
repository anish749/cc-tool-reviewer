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

func TestMatchesAll_CurlMultilineWithHead(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "head:*"},
	}

	cmd := `curl -s --max-time 15 'http://api.example.com/v1/items?format=json' -H 'Content-Type: application/json' -d '{
        "limit": 1,
        "filter": {"status": "active", "created_after": "2026-03-28T10:00:00Z"},
        "fields": ["id", "name", "updated_at"]
      }' | head -80`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("multiline curl with JSON body piped to head, both allowed, should match")
	}
}

func TestMatchesAll_CurlMultilineWithHeadCurlOnly(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
	}

	cmd := `curl -s --max-time 15 'http://api.example.com/v1/items?format=json' -H 'Content-Type: application/json' -d '{
        "limit": 1,
        "filter": {"status": "active", "created_after": "2026-03-28T10:00:00Z"},
        "fields": ["id", "name", "updated_at"]
      }' | head -80`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("head not in allow list → should NOT match")
	}
}

// --- Control-flow constructs (for, if, while, case) ---

func TestMatchesAll_ForLoopAllAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "python3:*"},
		{Tool: "Bash", Pattern: "[:*"},
	}

	cmd := `for region in us-east eu-west ap-south; do
        result=$(curl -s --max-time 10 "http://api.example.com/v1/regions/${region}/status" -H 'Content-Type: application/json' -d "{\"limit\":0}" 2>/dev/null)
        count=$(echo "$result" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['total'])" 2>/dev/null)
        if [ "$count" != "0" ] && [ -n "$count" ]; then
          echo "$region: $count hits"
        fi
      done`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("for loop with all inner commands allowed → should match")
	}
}

func TestMatchesAll_ForLoopPartiallyAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := `for region in us-east eu-west; do
        result=$(curl -s "http://example.com" 2>/dev/null)
        count=$(echo "$result" | python3 -c "import json,sys; print('ok')" 2>/dev/null)
        echo "$region: $count"
      done`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("python3 not in allow list → should NOT match")
	}
}

func TestMatchesAll_IfElseAllAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "[:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := `if [ -f go.mod ]; then echo found; else echo missing; fi`
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("if-else with all commands allowed → should match")
	}
}

func TestMatchesAll_IfElsePartiallyAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := `if [ -f go.mod ]; then echo found; else echo missing; fi`
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("[ (test condition) not in allow list → should NOT match")
	}
}

func TestMatchesAll_WhileAllAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "true:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "sleep:*"},
	}

	cmd := "while true; do echo waiting; sleep 1; done"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("while loop with all commands allowed → should match")
	}
}

func TestMatchesAll_WhilePartiallyAllowed(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "true:*"},
		{Tool: "Bash", Pattern: "echo:*"},
	}

	cmd := "while true; do echo waiting; sleep 1; done"
	got := MatchesAll("Bash", bashInput(cmd), rules)
	if got {
		t.Error("sleep not in allow list → should NOT match")
	}
}

func TestMatchesAny_DenyInsideForLoop(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "rm:*"}}

	cmd := "for f in /tmp/*.log; do rm -f $f; done"
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("denied command inside for loop → should match")
	}
}

func TestMatchesAny_DenyInsideIfBranch(t *testing.T) {
	denyRules := []Rule{{Tool: "Bash", Pattern: "rm:*"}}

	cmd := `if [ -f /tmp/data ]; then rm -rf /tmp/data; fi`
	got := MatchesAny("Bash", bashInput(cmd), denyRules)
	if !got {
		t.Error("denied command inside if-then → should match")
	}
}

func TestMatchesAll_CommentThenCurl(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
	}

	cmd := "# Fetch recent orders from the API\ncurl -s --max-time 15 'http://api.example.com/v1/orders?format=json' -H 'Content-Type: application/json' -d '{\n        \"limit\": 5,\n        \"filter\": {\"status\": \"pending\"},\n        \"fields\": [\"id\", \"amount\", \"created_at\"]\n      }'"

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("comment followed by curl, curl allowed → should match")
	}
}

func TestMatchesAll_CommentThenForLoop(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "python3:*"},
	}

	cmd := "# Check each region in parallel\nfor region in us-east eu-west; do\n  result=$(curl -s \"http://api.example.com/v1/regions/${region}/health\" 2>/dev/null)\n  count=$(echo \"$result\" | python3 -c \"import json,sys; print('ok')\" 2>/dev/null)\n  echo \"$region: $count\"\ndone"

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("comment + for-loop with all inner commands allowed → should match")
	}
}

func TestMatchesAll_CurlMultilineJSONBody(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "curl:*"},
	}

	cmd := `curl -s --max-time 15 'http://api.example.com/v1/users?format=json' -H 'Content-Type: application/json' -d '{
        "limit": 5,
        "filter": {"role": "admin"},
        "fields": ["id", "email", "created_at"]
      }'`

	got := MatchesAll("Bash", bashInput(cmd), rules)
	if !got {
		t.Error("standalone multiline curl with JSON body, curl allowed → should match")
	}
}

func TestMatchesAll_EmptyCommand(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "git:*"}}

	got := MatchesAll("Bash", bashInput(""), rules)
	if got {
		t.Error("empty command should NOT match any allow list")
	}
}

// TestDenyBeforeAllow verifies that deny rules take precedence over allow
// rules. The server checks deny first (server.go), so a specific deny like
// "git reset *" blocks even when a broad allow like "git:*" also matches.
//
// This test exercises the matcher functions that the server relies on to
// enforce that ordering.
// --- Shell command synonym handling ([ / [[ / test) ---

func TestInputSynonyms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"bracket to test", "[ -f foo ]", []string{"test -f foo ]"}},
		{"double bracket to test", "[[ -f foo ]]", []string{"test -f foo ]]"}},
		{"test to bracket and double bracket", "test -f foo", []string{"[ -f foo", "[[ -f foo"}},
		{"no synonym", "echo hello", nil},
		{"bare bracket", "[", []string{"test"}},
		{"bare test", "test", []string{"[", "[["}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inputSynonyms(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("inputSynonyms(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("inputSynonyms(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestMatchesRule_Synonyms(t *testing.T) {
	tests := []struct {
		name  string
		cmd   ParsedCommand
		rules []Rule
		want  bool
	}{
		{
			"bracket matches test:* via synonym",
			ParsedCommand{Text: `[ -f go.mod ]`, Args: []string{"[", "-f", "go.mod", "]"}},
			[]Rule{{Tool: "Bash", Pattern: "test:*"}},
			true,
		},
		{
			"test matches [:* via synonym",
			ParsedCommand{Text: "test -f go.mod", Args: []string{"test", "-f", "go.mod"}},
			[]Rule{{Tool: "Bash", Pattern: "[:*"}},
			true,
		},
		{
			"double bracket matches test:* via synonym",
			ParsedCommand{Text: `[[ -f go.mod ]]`},
			[]Rule{{Tool: "Bash", Pattern: "test:*"}},
			true,
		},
		{
			"test still matches test:* directly",
			ParsedCommand{Text: "test -f go.mod", Args: []string{"test", "-f", "go.mod"}},
			[]Rule{{Tool: "Bash", Pattern: "test:*"}},
			true,
		},
		{
			"unrelated command does not match via synonym",
			ParsedCommand{Text: "echo hello", Args: []string{"echo", "hello"}},
			[]Rule{{Tool: "Bash", Pattern: "test:*"}},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesRule("Bash", tc.cmd, tc.rules)
			if got != tc.want {
				t.Errorf("matchesRule(%q) = %v, want %v", tc.cmd.Text, got, tc.want)
			}
		})
	}
}

func TestMatchesAll_Synonyms(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		rules []Rule
		want  bool
	}{
		{
			"for loop with [ matches when test:* in allow list",
			"echo \"--- checking dirs ---\"\nfor d in a b c; do\n  echo \"== $d ==\"\n  ls -la \"/tmp/$d\" 2>/dev/null | head -5\n  if [ -f \"/tmp/$d/.git\" ]; then cat \"/tmp/$d/.git\"; fi\ndone",
			[]Rule{
				{Tool: "Bash", Pattern: "echo:*"},
				{Tool: "Bash", Pattern: "ls:*"},
				{Tool: "Bash", Pattern: "head:*"},
				{Tool: "Bash", Pattern: "cat:*"},
				{Tool: "Bash", Pattern: "test:*"},
			},
			true,
		},
		{
			"if-else with [ matches when test:* in allow list",
			`if [ -f go.mod ]; then echo found; else echo missing; fi`,
			[]Rule{
				{Tool: "Bash", Pattern: "echo:*"},
				{Tool: "Bash", Pattern: "test:*"},
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesAll("Bash", bashInput(tc.cmd), tc.rules)
			if got != tc.want {
				t.Errorf("MatchesAll = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesAny_DenySynonym(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		rules []Rule
		want  bool
	}{
		{
			"bracket caught by test:* deny rule",
			`if [ -f /tmp/secret ]; then echo found; fi`,
			[]Rule{{Tool: "Bash", Pattern: "test:*"}},
			true,
		},
		{
			"test caught by [:* deny rule",
			"test -f /tmp/secret && echo found",
			[]Rule{{Tool: "Bash", Pattern: "[:*"}},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesAny("Bash", bashInput(tc.cmd), tc.rules)
			if got != tc.want {
				t.Errorf("MatchesAny = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- Command normalization (git -C, --no-pager, etc.) ---

func TestMatchesAll_NormalizedGit(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		rules []Rule
		want  bool
	}{
		{
			"git -C with log rule",
			"git -C /foo log --oneline",
			[]Rule{{Tool: "Bash", Pattern: "git log *"}},
			true,
		},
		{
			"git -C with fetch rule",
			"git -C /foo fetch --prune",
			[]Rule{{Tool: "Bash", Pattern: "git fetch *"}},
			true,
		},
		{
			"git --no-pager with diff rule",
			"git --no-pager diff HEAD",
			[]Rule{{Tool: "Bash", Pattern: "git diff *"}},
			true,
		},
		{
			"git -C with config rule",
			`git -C /some/repo config user.email`,
			[]Rule{{Tool: "Bash", Pattern: "git config *"}},
			true,
		},
		{
			"compound with git -C",
			"git -C /foo fetch --prune 2>&1; echo done",
			[]Rule{
				{Tool: "Bash", Pattern: "git fetch *"},
				{Tool: "Bash", Pattern: "echo:*"},
			},
			true,
		},
		{
			"git -C does not match unrelated rule",
			"git -C /foo log --oneline",
			[]Rule{{Tool: "Bash", Pattern: "echo:*"}},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesAll("Bash", bashInput(tc.cmd), tc.rules)
			if got != tc.want {
				t.Errorf("MatchesAll = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesAny_DenyNormalizedGit(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		rules []Rule
		want  bool
	}{
		{
			"git -C reset caught by deny",
			"git -C /foo reset --hard HEAD",
			[]Rule{{Tool: "Bash", Pattern: "git reset *"}},
			true,
		},
		{
			"git -C push --force caught by deny",
			"git -C /foo push --force",
			[]Rule{{Tool: "Bash", Pattern: "git push *"}},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesAny("Bash", bashInput(tc.cmd), tc.rules)
			if got != tc.want {
				t.Errorf("MatchesAny = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesRule_NilArgsNoFalseMatch(t *testing.T) {
	// Non-CallExpr nodes (like [[ ... ]]) and non-Bash tools produce
	// Args == nil. normalize.Command(nil) returns "", which must not
	// be matched against patterns — otherwise any pattern with an
	// empty effective prefix (e.g. ":*", "**") would match everything.
	tests := []struct {
		name  string
		cmd   ParsedCommand
		rules []Rule
		want  bool
	}{
		{
			"nil args does not false-match via normalize",
			ParsedCommand{Text: `[[ -f foo ]]`},
			[]Rule{{Tool: "Bash", Pattern: "docker:*"}},
			false,
		},
		{
			"nil args does not match glob",
			ParsedCommand{Text: "/tmp/somefile"},
			[]Rule{{Tool: "Read", Pattern: "~/go/pkg/mod/**"}},
			false,
		},
		{
			"nil args Read input matches only by text",
			ParsedCommand{Text: "/tmp/somefile"},
			[]Rule{{Tool: "Read", Pattern: "/tmp/**"}},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := "Bash"
			if tc.rules[0].Tool != "Bash" {
				tool = tc.rules[0].Tool
			}
			got := matchesRule(tool, tc.cmd, tc.rules)
			if got != tc.want {
				t.Errorf("matchesRule(%q) = %v, want %v", tc.cmd.Text, got, tc.want)
			}
		})
	}
}

func TestDenyBeforeAllow(t *testing.T) {
	allow := []Rule{{Tool: "Bash", Pattern: "git:*"}}
	deny := []Rule{{Tool: "Bash", Pattern: "git reset *"}}

	tests := []struct {
		name       string
		cmd        string
		wantDeny   bool // MatchesAny(deny) — checked first by server
		wantAllow  bool // MatchesAll(allow) — checked second, skipped if denied
		wantResult string
	}{
		{
			name:       "allowed and not denied → locally allowed",
			cmd:        "git status",
			wantDeny:   false,
			wantAllow:  true,
			wantResult: "allow",
		},
		{
			name:       "denied even though allow also matches → locally denied",
			cmd:        "git reset --hard HEAD",
			wantDeny:   true,
			wantAllow:  true, // would match, but server never checks because deny came first
			wantResult: "deny",
		},
		{
			name:       "compound: one part denied → locally denied",
			cmd:        "git add . && git reset --hard",
			wantDeny:   true,
			wantAllow:  true,
			wantResult: "deny",
		},
		{
			name:       "neither denied nor allowed → sent to AI",
			cmd:        "docker build .",
			wantDeny:   false,
			wantAllow:  false,
			wantResult: "ai",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := bashInput(tc.cmd)

			denied := MatchesAny("Bash", input, deny)
			allowed := MatchesAll("Bash", input, allow)

			if denied != tc.wantDeny {
				t.Errorf("MatchesAny(deny) = %v, want %v", denied, tc.wantDeny)
			}
			if allowed != tc.wantAllow {
				t.Errorf("MatchesAll(allow) = %v, want %v", allowed, tc.wantAllow)
			}

			// Replicate the server's decision logic (server.go:105-111):
			// deny is checked first; if it matches, allow is never evaluated.
			var result string
			if denied {
				result = "deny"
			} else if allowed {
				result = "allow"
			} else {
				result = "ai"
			}
			if result != tc.wantResult {
				t.Errorf("server decision = %q, want %q", result, tc.wantResult)
			}
		})
	}
}

// An allow rule may only auto-match a normalized form that executes
// identically to the original command: normalization strips git global
// flags — and nothing else. Env assignments and redirects are part of
// what executes, so they must never be normalized away.
func TestMatchesRule_NormalizationMustNotHideExecution(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		pattern string
		want    bool
	}{
		// must NOT match: normalized form would hide part of what executes
		{"interleaved redirect is not stripped", "git > /tmp/x status", "git status", false},
		{"interleaved stderr redirect is not stripped", "git 2>/dev/null status", "git status", false},
		{"env assignment prefix is not stripped", "GIT_DIR=/elsewhere git push", "git push", false},
		{"env assignment plus global flag is not normalized", "FOO=bar git -C /x status", "git status", false},

		// must match: the behavior normalization exists for
		{"quoted flag value normalizes", `git -C "/path with spaces" status`, "git status", true},
		{"global flags before subcommand normalize", "git -C /foo --no-pager log", "git log:*", true},
		// Trailing redirects are outside the CallExpr span and never
		// reached matching; they must not disable normalization.
		{"trailing redirect keeps normalization", "git -C /foo status > /tmp/x", "git status", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := false
			for _, pc := range CollectAllCommands(tc.cmd) {
				if matchesRule("Bash", pc, []Rule{{Tool: "Bash", Pattern: tc.pattern}}) {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("match(%q, pattern %q) = %v, want %v", tc.cmd, tc.pattern, got, tc.want)
			}
		})
	}
}

// --- Monitor tool (shell script embedded in the "command" field) ---

func monitorInput(command string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"command":     command,
		"description": "watch for errors",
		"timeout_ms":  60000,
		"persistent":  false,
	})
	return b
}

func TestMatchesAll_MonitorAllowedByBashRules(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "tail:*"},
		{Tool: "Bash", Pattern: "grep:*"},
	}

	got := MatchesAll("Monitor", monitorInput("tail -f app.log | grep --line-buffered ERROR"), rules)
	if !got {
		t.Error("Monitor command composed of allowed Bash commands should match")
	}
}

func TestMatchesAll_MonitorPartial(t *testing.T) {
	rules := []Rule{{Tool: "Bash", Pattern: "tail:*"}}

	got := MatchesAll("Monitor", monitorInput("tail -f app.log | grep ERROR"), rules)
	if got {
		t.Error("Monitor command with unmatched grep should NOT match")
	}
}

func TestMatchesAll_MonitorLoopWithSubshell(t *testing.T) {
	rules := []Rule{
		{Tool: "Bash", Pattern: "/Users/anish/cb/claude_utils/clu:*"},
		{Tool: "Bash", Pattern: "grep:*"},
		{Tool: "Bash", Pattern: "echo:*"},
		{Tool: "Bash", Pattern: "seq:*"},
		{Tool: "Bash", Pattern: "sleep:*"},
		{Tool: "Bash", Pattern: "test:*"},
	}

	cmd := "for i in $(seq 1 90); do\n  ip=$(/Users/anish/cb/claude_utils/clu get-last-notebook -no-open 2>&1 | grep -iE 'Jupyter')\n  if [ -n \"$ip\" ]; then echo \"$ip\"; break; fi\n  sleep 10\ndone"
	got := MatchesAll("Monitor", monitorInput(cmd), rules)
	if !got {
		t.Error("Monitor polling loop of allowed commands should match")
	}
}

func TestMatchesAny_MonitorDeniedByBashRules(t *testing.T) {
	deny := []Rule{{Tool: "Bash", Pattern: "git push:*"}}

	cmd := "while true; do git push --force; sleep 5; done"
	if !MatchesAny("Monitor", monitorInput(cmd), deny) {
		t.Error("Bash deny rule should catch command embedded in Monitor")
	}
}

func TestMatchesAll_MonitorWebSocketVariant(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"description": "deploy events",
		"ws":          map[string]any{"url": "wss://example.com/stream"},
	})
	rules := []Rule{
		{Tool: "Bash", Pattern: "*"},
		{Tool: "Monitor", Pattern: "*"},
	}

	if MatchesAll("Monitor", b, rules) {
		t.Error("ws Monitor has no command to inspect; must fall through to review")
	}
}

func TestMatchesAll_BashNotGovernedByMonitorRules(t *testing.T) {
	rules := []Rule{{Tool: "Monitor", Pattern: "rm:*"}}

	if MatchesAll("Bash", bashInput("rm -rf /tmp/x"), rules) {
		t.Error("Monitor rules must not govern Bash commands")
	}
}
