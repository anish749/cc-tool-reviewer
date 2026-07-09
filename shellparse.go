package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ParsedCommand holds both the original source text of a shell command
// and its individual arguments as parsed by the shell AST.
type ParsedCommand struct {
	Text string   // original source text for pattern matching
	Args []string // individual arguments from the AST (nil for non-CallExpr nodes)
}

// CollectAllCommands returns every simple command that would execute from
// a potentially compound, potentially nested shell command string.
//
// It parses the command with mvdan.cc/sh and walks the full AST:
//   - All binary operators (|, &&, ||) split into separate commands
//   - for, if, while, case, subshell, and block constructs are descended into
//   - $() and backtick subshells are recursively descended into
//
// Each command carries its original source text (for prefix matching)
// and its parsed arguments (for flag-aware normalization).
func CollectAllCommands(cmd string) []ParsedCommand {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return []ParsedCommand{{Text: cmd}}
	}
	var cmds []ParsedCommand
	for _, stmt := range f.Stmts {
		collectStmt(stmt, cmd, &cmds)
	}
	return cmds
}

// collectStmt recursively collects command strings from a statement.
func collectStmt(stmt *syntax.Stmt, src string, out *[]ParsedCommand) {
	if stmt == nil || stmt.Cmd == nil {
		return
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
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
			collectCmdSubsts(cmd, src, out)
		} else {
			addCallExpr(cmd, src, out)
			collectCmdSubsts(cmd, src, out)
		}
	default:
		addNodeText(stmt.Cmd, src, out)
		collectCmdSubsts(stmt.Cmd, src, out)
	}
}

// collectStmts collects commands from a slice of statements.
func collectStmts(stmts []*syntax.Stmt, src string, out *[]ParsedCommand) {
	for _, s := range stmts {
		collectStmt(s, src, out)
	}
}

// collectIfClause recursively collects commands from if/elif/else chains.
func collectIfClause(ic *syntax.IfClause, src string, out *[]ParsedCommand) {
	collectStmts(ic.Cond, src, out)
	collectStmts(ic.Then, src, out)
	if ic.Else != nil {
		collectIfClause(ic.Else, src, out)
	}
}

// addCallExpr extracts both the source text and individual argument texts
// from a CallExpr node.
func addCallExpr(call *syntax.CallExpr, src string, out *[]ParsedCommand) {
	start := int(call.Pos().Offset())
	end := int(call.End().Offset())
	if start >= len(src) || end > len(src) || start >= end {
		return
	}
	text := strings.TrimSpace(src[start:end])
	if text == "" {
		return
	}

	var args []string
	for _, w := range call.Args {
		ws := int(w.Pos().Offset())
		we := int(w.End().Offset())
		if ws < len(src) && we <= len(src) && ws < we {
			args = append(args, src[ws:we])
		}
	}

	*out = append(*out, ParsedCommand{Text: text, Args: args})
}

// addNodeText extracts the original source text for a node and appends
// it as a ParsedCommand with no parsed args.
func addNodeText(node syntax.Node, src string, out *[]ParsedCommand) {
	start := int(node.Pos().Offset())
	end := int(node.End().Offset())
	if start >= len(src) || end > len(src) || start >= end {
		return
	}
	if text := strings.TrimSpace(src[start:end]); text != "" {
		*out = append(*out, ParsedCommand{Text: text})
	}
}

// collectCmdSubsts walks a node and recursively collects commands from
// any $() or backtick command substitutions found.
func collectCmdSubsts(node syntax.Node, src string, out *[]ParsedCommand) {
	syntax.Walk(node, func(n syntax.Node) bool {
		sub, ok := n.(*syntax.CmdSubst)
		if !ok {
			return true
		}
		for _, stmt := range sub.Stmts {
			collectStmt(stmt, src, out)
		}
		return false
	})
}
