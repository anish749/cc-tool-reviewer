package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// isCompoundCommand reports whether a bash command is more than a single
// simple command — i.e. it contains pipelines (|), command lists (&&, ||),
// separators (;, newline), or command substitutions ($(), backticks).
//
// When true, MatchesAll/MatchesAny will decompose the command and check
// every sub-command individually against the rules.
func isCompoundCommand(cmd string) bool {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return false
	}
	if len(f.Stmts) != 1 {
		return len(f.Stmts) > 1
	}
	return stmtIsCompound(f.Stmts[0])
}

// stmtIsCompound checks whether a single statement contains any
// binary operators (|, &&, ||) or command substitutions.
func stmtIsCompound(stmt *syntax.Stmt) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		return true // |, &&, ||
	case *syntax.CallExpr:
		// Simple command — only compound if args contain $() or backticks
		found := false
		syntax.Walk(cmd, func(node syntax.Node) bool {
			if _, ok := node.(*syntax.CmdSubst); ok {
				found = true
				return false
			}
			return !found
		})
		return found
	default:
		return true
	}
}

// CollectAllCommands returns every simple command that would execute from
// a potentially compound, potentially nested shell command string.
//
// It parses the command with mvdan.cc/sh and walks the full AST:
//   - All binary operators (|, &&, ||) split into separate commands
//   - $() and backtick subshells are recursively descended into
//
// Each command is represented as its original source text so that
// matchPattern can do prefix matching (e.g. "curl:*").
//
// Example: "echo $(git status && date)" returns:
//
//	["echo $(git status && date)", "git status", "date"]
func CollectAllCommands(cmd string) []string {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return []string{cmd} // unparseable → return as-is
	}
	var cmds []string
	for _, stmt := range f.Stmts {
		collectStmt(stmt, cmd, &cmds)
	}
	return cmds
}

// collectStmt recursively collects command strings from a statement.
func collectStmt(stmt *syntax.Stmt, src string, out *[]string) {
	if stmt == nil || stmt.Cmd == nil {
		return
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		// |, &&, || — collect each side separately
		collectStmt(cmd.X, src, out)
		collectStmt(cmd.Y, src, out)
	default:
		// Simple command, subshell, block, etc.
		// Use stmt.Cmd (not stmt) to exclude semicolons from the text.
		addNodeText(stmt.Cmd, src, out)
		collectCmdSubsts(stmt.Cmd, src, out)
	}
}

// addNodeText extracts the original source text for a node and appends it.
func addNodeText(node syntax.Node, src string, out *[]string) {
	start := int(node.Pos().Offset())
	end := int(node.End().Offset())
	if start >= len(src) || end > len(src) || start >= end {
		return
	}
	if text := strings.TrimSpace(src[start:end]); text != "" {
		*out = append(*out, text)
	}
}

// collectCmdSubsts walks a node and recursively collects commands from
// any $() or backtick command substitutions found.
func collectCmdSubsts(node syntax.Node, src string, out *[]string) {
	syntax.Walk(node, func(n syntax.Node) bool {
		sub, ok := n.(*syntax.CmdSubst)
		if !ok {
			return true // keep descending
		}
		for _, stmt := range sub.Stmts {
			collectStmt(stmt, src, out)
		}
		return false // don't re-descend into this CmdSubst's children
	})
}
