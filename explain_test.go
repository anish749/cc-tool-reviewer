package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anish/cc-tool-reviewer/internal/reviewlog"
)

func mkRules(t *testing.T, raws ...string) []Rule {
	t.Helper()
	var rs []Rule
	for _, raw := range raws {
		r, ok := ParseRule(raw)
		if !ok {
			t.Fatalf("bad rule %q", raw)
		}
		rs = append(rs, r)
	}
	return rs
}

func bashEntry(cmd string) reviewlog.Entry {
	in, _ := json.Marshal(map[string]string{"command": cmd})
	return reviewlog.Entry{ToolName: "Bash", ToolInput: in, Decision: "ask"}
}

func labels(d diagnosis) []string {
	out := make([]string, len(d.Unmatched))
	for i, c := range d.Unmatched {
		out[i] = c.Label
	}
	return out
}

func hasLabel(d diagnosis, want string) bool {
	for _, c := range d.Unmatched {
		if c.Label == want {
			return true
		}
	}
	return false
}

func TestExplainEntry_PrintfCulprit(t *testing.T) {
	allow := mkRules(t, "Bash(rg:*)", "Bash(head:*)")
	e := bashEntry(`rg -n 'x' --type go | head -20; printf '=====\n'; rg -n 'y' | head -20`)

	d := explainEntry(e, allow, nil)

	if d.Class != classStillFails {
		t.Fatalf("class = %v, want classStillFails", d.Class)
	}
	if !hasLabel(d, "printf") {
		t.Fatalf("unmatched = %v, want to contain printf", labels(d))
	}
	// printf appears twice in the command but must be deduped to one culprit.
	if got := len(d.Unmatched); got != 1 {
		t.Fatalf("unmatched count = %d (%v), want 1 deduped culprit", got, labels(d))
	}
}

func TestExplainEntry_NowAllowed(t *testing.T) {
	allow := mkRules(t, "Bash(ls:*)", "Bash(wc:*)", "Bash(echo:*)")
	e := bashEntry(`ls -la | wc -l; echo "done"`)

	d := explainEntry(e, allow, nil)

	if d.Class != classNowAllowed {
		t.Fatalf("class = %v, want classNowAllowed", d.Class)
	}
}

func TestExplainEntry_NonBashToolLevel(t *testing.T) {
	allow := mkRules(t, "Bash(rg:*)")
	in, _ := json.Marshal(map[string]any{"command": "tail -f x.log", "description": "watch"})
	e := reviewlog.Entry{ToolName: "Monitor", ToolInput: in, Decision: "ask"}

	d := explainEntry(e, allow, nil)

	if d.Class != classStillFails {
		t.Fatalf("class = %v, want classStillFails", d.Class)
	}
	if !hasLabel(d, "Monitor (tool-level)") {
		t.Fatalf("unmatched = %v, want Monitor (tool-level)", labels(d))
	}
}

func TestExplainEntry_NowDenied(t *testing.T) {
	allow := mkRules(t, "Bash(git status *)")
	deny := mkRules(t, "Bash(git reset *)")
	e := bashEntry(`git reset --hard HEAD~1`)

	d := explainEntry(e, allow, deny)

	if d.Class != classNowDenied {
		t.Fatalf("class = %v, want classNowDenied", d.Class)
	}
}

func TestExplainEntry_GitSubcommandLabel(t *testing.T) {
	allow := mkRules(t, "Bash(git status *)", "Bash(git log *)")
	e := bashEntry(`git rebase -i origin/main`)

	d := explainEntry(e, allow, nil)

	if d.Class != classStillFails {
		t.Fatalf("class = %v, want classStillFails", d.Class)
	}
	if !hasLabel(d, "git rebase") {
		t.Fatalf("unmatched = %v, want git rebase", labels(d))
	}
}

func TestPrintReport_NoReplay(t *testing.T) {
	allow := mkRules(t, "Bash(ls:*)")
	entries := []reviewlog.Entry{bashEntry("curl http://example.com")}

	var buf bytes.Buffer
	printReport(&buf, []string{"test.log"}, entries, nil, allow, nil, nil)

	out := buf.String()
	if !strings.Contains(out, "STILL GO TO llm-reviewed") {
		t.Fatal("missing still-fails section")
	}
	if strings.Contains(out, "replayed through LLM reviewer") {
		t.Fatal("replay output should not appear without --replay")
	}
}

func TestPrintReport_WithReplay(t *testing.T) {
	allow := mkRules(t, "Bash(ls:*)")
	entries := []reviewlog.Entry{bashEntry("curl http://example.com")}

	reviewer := NewReviewer(&stubLLMClient{response: `{"decision":"allow","reason":"test reason"}`}, nil)

	var buf bytes.Buffer
	printReport(&buf, []string{"test.log"}, entries, nil, allow, nil, reviewer)

	out := buf.String()
	if !strings.Contains(out, `replayed through LLM reviewer: allow`) {
		t.Fatalf("missing replay decision in output:\n%s", out)
	}
	if !strings.Contains(out, `"test reason"`) {
		t.Fatalf("missing replay reason in output:\n%s", out)
	}
	if !strings.Contains(out, "Replayed 1 through LLM reviewer (1 allow, 0 ask, 0 error") {
		t.Fatalf("missing replay summary in output:\n%s", out)
	}
}

// stubLLMClient is a test double for llm.Client that returns a canned response.
type stubLLMClient struct {
	response string
}

func (s *stubLLMClient) Text(_ context.Context, _, _ string) (string, error) {
	return s.response, nil
}

func (s *stubLLMClient) JSON(_ context.Context, _, _ string, out any) error {
	return json.Unmarshal([]byte(s.response), out)
}

func TestParseEntries(t *testing.T) {
	input := strings.Join([]string{
		`{"ts":"t1","tool":"Bash","input":{"command":"ls"},"decision":"allow","reason":"r1"}`,
		``, // blank line, skipped
		`{"ts":"t2","tool":"Bash","input":{"command":"rg x"},"decision":"ask","reason":"r2"}`,
		`{not valid json}`, // malformed, counted as an error
	}, "\n")

	entries, errs := parseEntries(strings.NewReader(input))

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if len(errs) != 1 {
		t.Fatalf("errs = %d, want 1", len(errs))
	}
	if entries[0].Timestamp != "t1" || entries[1].Timestamp != "t2" {
		t.Fatalf("timestamps = %q, %q", entries[0].Timestamp, entries[1].Timestamp)
	}
}
