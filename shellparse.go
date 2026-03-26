package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// isCompoundCommand reports whether a bash command contains shell operators
// (command separators, subshells) outside of quoted strings.
//
// Uses mvdan.cc/sh to parse the command into an AST. Pipelines (|) are
// intentionally NOT considered compound — they are one logical command
// and can be matched by prefix rules like "curl:*".
func isCompoundCommand(cmd string) bool {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return false
	}
	if len(f.Stmts) > 1 {
		return true
	}
	if len(f.Stmts) == 0 {
		return false
	}
	return stmtIsCompound(f.Stmts[0])
}

// stmtIsCompound checks whether a single statement is compound.
// Pipes are NOT compound — we recurse into both sides to check for
// actual compound operators or subshells.
func stmtIsCompound(stmt *syntax.Stmt) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
			// Pipeline — not compound itself, but check each side
			return stmtIsCompound(cmd.X) || stmtIsCompound(cmd.Y)
		}
		return true // &&, ||
	case *syntax.CallExpr:
		return callHasSubshell(cmd)
	default:
		// Subshell, Block, IfClause, ForClause, etc.
		return true
	}
}

// callHasSubshell reports whether a simple command contains $() or
// backtick substitutions anywhere in its arguments.
func callHasSubshell(call *syntax.CallExpr) bool {
	found := false
	syntax.Walk(call, func(node syntax.Node) bool {
		if found {
			return false
		}
		if _, ok := node.(*syntax.CmdSubst); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

// CollectAllCommands returns every simple command that would execute from
// a potentially compound, potentially nested shell command string.
//
// It parses the command with mvdan.cc/sh and walks the full AST:
//   - && and || operators split into separate commands
//   - Pipelines (|) are kept as one command string
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
		if cmd.Op == syntax.Pipe || cmd.Op == syntax.PipeAll {
			// Pipeline — keep as one command string
			addNodeText(cmd, src, out)
			collectCmdSubsts(cmd, src, out)
		} else {
			// && or || — collect each side separately
			collectStmt(cmd.X, src, out)
			collectStmt(cmd.Y, src, out)
		}
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
