package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// CollectAllCommands returns every simple command that would execute from
// a potentially compound, potentially nested shell command string.
//
// It parses the command with mvdan.cc/sh and walks the full AST:
//   - All binary operators (|, &&, ||) split into separate commands
//   - for, if, while, case, subshell, and block constructs are descended into
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
	case *syntax.ForClause:
		collectStmts(cmd.Do, src, out)
	case *syntax.WhileClause:
		collectStmts(cmd.Cond, src, out)
		collectStmts(cmd.Do, src, out)
	case *syntax.IfClause:
		collectIfClause(cmd, src, out)
	case *syntax.CaseClause:
		for _, ci := range cmd.Items {
			collectStmts(ci.Stmts, src, out)
		}
	case *syntax.Subshell:
		collectStmts(cmd.Stmts, src, out)
	case *syntax.Block:
		collectStmts(cmd.Stmts, src, out)
	case *syntax.CallExpr:
		if len(cmd.Args) == 0 && len(cmd.Assigns) > 0 {
			// Pure assignment (e.g. result=$(curl ...)) — not a command
			// to gate, but descend into subshells in the values.
			collectCmdSubsts(cmd, src, out)
		} else {
			addNodeText(cmd, src, out)
			collectCmdSubsts(cmd, src, out)
		}
	default:
		addNodeText(stmt.Cmd, src, out)
		collectCmdSubsts(stmt.Cmd, src, out)
	}
}

// collectStmts collects commands from a slice of statements.
func collectStmts(stmts []*syntax.Stmt, src string, out *[]string) {
	for _, s := range stmts {
		collectStmt(s, src, out)
	}
}

// collectIfClause recursively collects commands from if/elif/else chains.
func collectIfClause(ic *syntax.IfClause, src string, out *[]string) {
	collectStmts(ic.Cond, src, out)
	collectStmts(ic.Then, src, out)
	if ic.Else != nil {
		collectIfClause(ic.Else, src, out)
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
