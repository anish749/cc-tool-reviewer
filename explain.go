package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anish/cc-tool-reviewer/internal/reviewlog"
)

// entryClass is the verdict of replaying a logged tool call through the
// current static rules.
type entryClass int

const (
	// classStillFails: MatchesAll(allow) is false today, so this call would
	// still fall through to the LLM reviewer. Unmatched holds the culprits.
	classStillFails entryClass = iota
	// classNowAllowed: MatchesAll(allow) is true today — the rules have since
	// been extended to cover it, so it would no longer reach the LLM.
	classNowAllowed
	// classNowDenied: MatchesAny(deny) is true today. Denied calls are never
	// logged, so this means the rules changed since; flagged as an anomaly.
	classNowDenied
)

// culprit is a sub-command (or, for non-Bash tools, the whole call) that has
// no matching allow rule. Label is the grouping key for the summary; Text is
// the full sub-command source for the per-entry detail.
type culprit struct {
	Label string
	Text  string
}

// diagnosis is the result of replaying one logged entry against current rules.
type diagnosis struct {
	Class     entryClass
	Unmatched []culprit // deduped by Label; populated for classStillFails
}

// explainEntry replays a single logged tool call through the current static
// allow/deny rules and reports why it did (or no longer would) fall through to
// the LLM reviewer. It mirrors the server's order: deny first, then allow.
func explainEntry(e reviewlog.Entry, allow, deny []Rule) diagnosis {
	if MatchesAny(e.ToolName, e.ToolInput, deny) {
		return diagnosis{Class: classNowDenied, Unmatched: unmatchedAgainst(e, deny)}
	}
	if MatchesAll(e.ToolName, e.ToolInput, allow) {
		return diagnosis{Class: classNowAllowed}
	}
	return diagnosis{Class: classStillFails, Unmatched: unmatchedAgainst(e, allow)}
}

// unmatchedAgainst returns the sub-commands of a tool call that do NOT match
// any of the given rules, deduped by label. For classNowDenied it is called
// with the deny rules to report which sub-commands the deny caught.
func unmatchedAgainst(e reviewlog.Entry, rules []Rule) []culprit {
	var out []culprit
	seen := make(map[string]bool)
	for _, cmd := range toolCommands(e.ToolName, e.ToolInput) {
		if matchesRule(e.ToolName, cmd, rules) {
			continue
		}
		label := culpritLabel(e.ToolName, cmd)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, culprit{Label: label, Text: cmd.Text})
	}
	return out
}

// culpritLabel returns the grouping key for a sub-command: the tool name for
// non-Bash calls (matched opaquely), otherwise the leading token — with git
// widened to two tokens, since git rules are subcommand-specific.
func culpritLabel(toolName string, cmd ParsedCommand) string {
	if toolName != "Bash" {
		return toolName + " (tool-level)"
	}
	fields := cmd.Args
	if len(fields) == 0 {
		fields = strings.Fields(cmd.Text)
	}
	if len(fields) == 0 {
		return cmd.Text
	}
	lead := fields[0]
	if filepath.Base(lead) == "git" && len(fields) > 1 {
		return lead + " " + fields[1]
	}
	return lead
}

// parseEntries reads JSONL review-log entries, one per line. Blank lines are
// skipped; malformed lines are collected as errors rather than aborting.
func parseEntries(r io.Reader) ([]reviewlog.Entry, []error) {
	var entries []reviewlog.Entry
	var errs []error
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		b := strings.TrimSpace(sc.Text())
		if b == "" {
			continue
		}
		var e reviewlog.Entry
		if err := json.Unmarshal([]byte(b), &e); err != nil {
			errs = append(errs, fmt.Errorf("line %d: %w", line, err))
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return entries, errs
}

// readEntries reads and parses one log file, transparently decompressing
// gzip-compressed rotated backups (*.gz).
func readEntries(path string) ([]reviewlog.Entry, []error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		defer gz.Close()
		r = gz
	}
	entries, errs := parseEntries(r)
	return entries, errs, nil
}

// resolveLogFiles expands the given args (file paths) into a sorted file list.
// With no args it defaults to the rotating log family "llm-logs*" in the cwd.
func resolveLogFiles(args []string) ([]string, error) {
	if len(args) == 0 {
		matches, err := filepath.Glob("llm-logs*")
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no log files given and no ./llm-logs* found; pass the log path explicitly")
		}
		sort.Strings(matches)
		return matches, nil
	}
	return args, nil
}

// runExplain is the entry point for the `explain` subcommand.
func runExplain(args []string) error {
	files, err := resolveLogFiles(args)
	if err != nil {
		return err
	}

	allow, deny, _ := LoadRules()

	var entries []reviewlog.Entry
	var parseErrs []error
	for _, path := range files {
		got, errs, err := readEntries(path)
		if err != nil {
			return err
		}
		entries = append(entries, got...)
		parseErrs = append(parseErrs, errs...)
	}

	printReport(os.Stdout, files, entries, parseErrs, allow, deny)
	return nil
}

// printReport renders the per-entry root cause. Every logged call reached the
// llm-reviewed stage because static-check couldn't resolve it; for each one that
// STILL can't be resolved by the current rules, it names the sub-command that no
// allow rule matches (the reason MatchesAll fails). Calls the current rules now
// resolve are collapsed to a single count.
func printReport(w io.Writer, files []string, entries []reviewlog.Entry, parseErrs []error, allow, deny []Rule) {
	diags := make([]diagnosis, len(entries))
	var stillFails []int
	nowAllowed, nowDenied := 0, 0

	for i, e := range entries {
		d := explainEntry(e, allow, deny)
		diags[i] = d
		switch d.Class {
		case classStillFails:
			stillFails = append(stillFails, i)
		case classNowAllowed:
			nowAllowed++
		case classNowDenied:
			nowDenied++
		}
	}

	fmt.Fprintf(w, "llm-review log — %d entries from %d file%s\n", len(entries), len(files), plural(len(files)))
	fmt.Fprintln(w, "Every line is a call that static-check neither allowed nor denied, so it fell")
	fmt.Fprintln(w, "through to the llm-reviewed stage (a Haiku call, sometimes a prompt).")
	fmt.Fprintf(w, "Replayed against your current rules: %d are now resolved by static-check and\n", nowAllowed+nowDenied)
	fmt.Fprintf(w, "omitted (%d allow, %d deny); %d still fall through, each shown with the\n", nowAllowed, nowDenied, len(stillFails))
	fmt.Fprintln(w, "sub-command that no allow rule matches.")
	fmt.Fprintln(w, "(Project-level rules aren't replayed — the log records no CWD.)")
	if len(parseErrs) > 0 {
		fmt.Fprintf(w, "Skipped %d unparseable log line%s.\n", len(parseErrs), plural(len(parseErrs)))
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "STILL GO TO llm-reviewed UNDER CURRENT RULES (%d)\n", len(stillFails))
	if len(stillFails) == 0 {
		fmt.Fprintln(w, "  none — every logged fallback is now resolved by static-check.")
		return
	}
	for _, i := range stillFails {
		e := entries[i]
		fmt.Fprintf(w, "\n  %s  %s\n", e.Timestamp, e.ToolName)
		fmt.Fprintf(w, "    [logged]  llm decided: %s\n", e.Decision)
		if e.ToolName != "Bash" || len(toolCommands(e.ToolName, e.ToolInput)) > 1 {
			fmt.Fprintln(w, "    [logged]  full command:")
			fmt.Fprintln(w, indent(commandOf(e), "              "))
		} else {
			fmt.Fprintf(w, "    [logged]  command: %s\n", commandOf(e))
		}
		fmt.Fprintln(w, "    [replay]  no allow rule matches:")
		for _, c := range diags[i].Unmatched {
			fmt.Fprintln(w, indent(blockerText(e.ToolName, c), "              "))
		}
	}
}

// blockerText is the sub-command that no allow rule matched — the concrete
// reason the call fell through — shown verbatim. Non-Bash tools are matched
// opaquely, so it names the tool instead.
func blockerText(toolName string, c culprit) string {
	if toolName != "Bash" {
		return "tool " + toolName + " (no allow rule for this tool)"
	}
	return c.Text
}

// commandOf returns the Bash command string for display, or the raw tool input
// for non-Bash tools.
func commandOf(e reviewlog.Entry) string {
	return ToolInputString(e.ToolName, e.ToolInput)
}

// indent prefixes every line of s, so a multi-line command stays aligned under
// its heading without anything being collapsed or truncated.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
