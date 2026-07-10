package main

// A/B live test for the reviewer prompt. Reconstructs a logged failure (a Bash
// tool call whose description field reads like an instruction) and runs the
// old and new prompt variants abRuns times each against the live model,
// counting JSON verdicts vs. conversational replies (parse failures). Skipped
// in short mode:
//
//	go test . -run TestLive_PromptAB -v

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anish/cc-tool-reviewer/internal/llm"
)

const abRuns = 10

var abAllowRules = []string{"Bash(rg:*)", "Bash(head:*)", "Bash(ls:*)", "Bash(go test:*)"}

// abToolInput mirrors the tool call behind the log's parse failure: a composed
// rg search whose description reads like a user instruction.
var abToolInput = json.RawMessage(`{"command":"rg -n 'gonum' --type go | head; printf '=====\n'; rg -n 'cosine|dot|norm|similarity' -i --type go | rg -v '_test|normal' | head -20; printf '=====\n'; rg -n '\"0123456789|abcdefghijklmnop' --type go | head","description":"Check gonum usage, vector math implementations, and alphabet constants"}`)

// oldSystemPrompt and oldUserMessage are verbatim copies of the pre-change
// prompt construction.
func oldSystemPrompt(allowRules []string) string {
	var sb strings.Builder
	sb.WriteString(`You are reviewing tool calls for a CLI tool called Claude Code. A tool call is about to execute that did not exactly match the user's configured permission rules. Your job is to reduce unnecessary prompts by allowing commands that are consistent with what the user has already permitted.

The user has explicitly allowed the following patterns:
`)
	for _, rule := range allowRules {
		sb.WriteString("- ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}
	sb.WriteString(`
Default to "allow". Only respond "ask" if you cannot find any reasonable connection between the command and what the user has already allowed.

A command should be allowed if ANY of these are true:
- It is a composition of allowed commands (pipes, &&, ||, ;, subshells, multi-line scripts)
- It is a variation of an allowed pattern (different flags, arguments, or targets)
- It is a read-only command that merely observes state (pgrep, whoami, which, ps, lsof, date, wc, du, df, uptime, file, type, env, printenv, id, hostname, uname, sw_vers, etc.)
- It is a standard development command that a developer using the allowed tools would reasonably also use

Only evaluate commands actually EXECUTED by the shell, not strings inside quotes, echo arguments, or data literals.

Respond with ONLY a valid JSON object. No markdown, no explanation, no code fences:
{"decision": "allow" or "ask", "reason": "brief one-line reason"}`)
	return sb.String()
}

func oldUserMessage(toolName string, toolInput json.RawMessage) string {
	return fmt.Sprintf("Tool: %s\nInput: %s", toolName, string(toolInput))
}

func TestLive_PromptAB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found on PATH")
	}

	client := llm.NewCLIClient(llm.WithModel(reviewModel))

	variants := []struct {
		name   string
		system string
		user   string
	}{
		{"old", oldSystemPrompt(abAllowRules), oldUserMessage("Bash", abToolInput)},
		{"new", buildSystemPrompt(abAllowRules), buildUserMessage("Bash", abToolInput)},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			var mu sync.Mutex
			var jsonReplies, conversational int
			decisions := map[string]int{}
			var wg sync.WaitGroup
			for i := range abRuns {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
					defer cancel()
					var d ReviewDecision
					err := client.JSON(ctx, v.system, v.user, &d)
					mu.Lock()
					defer mu.Unlock()
					var pe *llm.ParseError
					switch {
					case errors.As(err, &pe):
						conversational++
						t.Logf("[%s run %d] NOT JSON: %.400q", v.name, i, pe.Raw)
					case err != nil:
						t.Errorf("[%s run %d] transport error: %v", v.name, i, err)
					default:
						jsonReplies++
						decisions[strings.ToLower(strings.TrimSpace(d.Decision))]++
						t.Logf("[%s run %d] decision=%q reason=%q", v.name, i, d.Decision, d.Reason)
					}
				}(i)
			}
			wg.Wait()
			t.Logf("[%s] SUMMARY: json=%d/%d conversational=%d/%d decisions=%v",
				v.name, jsonReplies, abRuns, conversational, abRuns, decisions)
		})
	}
}
