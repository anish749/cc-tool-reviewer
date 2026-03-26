package main

import "strings"

// isCompoundCommand reports whether a bash command contains shell operators
// (command separators, subshells) outside of quoted strings.
//
// Operators checked:
//   - \n, ;       — command separators
//   - &&, ||      — list operators
//   - $(), ``     — command substitution (flagged even inside double quotes,
//     since they expand there; only single quotes suppress)
//
// Single pipe (|) is intentionally NOT flagged — a pipeline is one logical
// command and can be matched by prefix rules like "curl:*".
func isCompoundCommand(cmd string) bool {
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			if c == '\\' && i+1 < len(cmd) {
				i++ // skip escaped char
				continue
			}
			if c == '"' {
				inDouble = false
				continue
			}
			// $() and backticks expand inside double quotes
			if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
				return true
			}
			if c == '`' {
				return true
			}
			continue
		}

		// Outside quotes
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(cmd) {
				i++ // skip escaped char
			}
		case '\n', ';':
			return true
		case '&':
			if i+1 < len(cmd) && cmd[i+1] == '&' {
				return true
			}
		case '|':
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				return true
			}
		case '$':
			if i+1 < len(cmd) && cmd[i+1] == '(' {
				return true
			}
		case '`':
			return true
		}
	}
	return false
}

// SplitCommands splits a compound shell command into individual commands
// by splitting on operators (&&, ||, ;, newline) that appear outside of
// quoted strings.
//
// Single pipes (|) are NOT treated as separators — a pipeline like
// "cmd1 | cmd2" is kept as one command string, because pipes chain
// data flow rather than sequencing independent commands.
//
// Returns trimmed, non-empty command strings.
func SplitCommands(cmd string) []string {
	var commands []string
	var cur strings.Builder
	inSingle := false
	inDouble := false

	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			commands = append(commands, s)
		}
		cur.Reset()
	}

	// writeSubshell skips over a $() subshell, writing the whole thing
	// (including delimiters) into cur without splitting inside it.
	writeSubshell := func(pos int) int {
		cur.WriteByte('$')
		cur.WriteByte('(')
		content, end := extractParenContent(cmd, pos+2)
		if end >= 0 {
			cur.WriteString(content)
			cur.WriteByte(')')
			return end
		}
		return pos + 1 // unmatched, skip just the (
	}

	// writeBacktick skips over a `...` subshell.
	writeBacktick := func(pos int) int {
		cur.WriteByte('`')
		content, end := extractBacktickContent(cmd, pos+1)
		if end >= 0 {
			cur.WriteString(content)
			cur.WriteByte('`')
			return end
		}
		return pos // unmatched
	}

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		if inSingle {
			cur.WriteByte(c)
			if c == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			if c == '\\' && i+1 < len(cmd) {
				cur.WriteByte(c)
				i++
				cur.WriteByte(cmd[i])
				continue
			}
			if c == '"' {
				cur.WriteByte(c)
				inDouble = false
				continue
			}
			// $() and backticks expand inside double quotes — skip over them
			if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
				i = writeSubshell(i)
				continue
			}
			if c == '`' {
				i = writeBacktick(i)
				continue
			}
			cur.WriteByte(c)
			continue
		}

		// Outside quotes — check for separators before appending
		switch {
		case c == '\'':
			inSingle = true
			cur.WriteByte(c)
		case c == '"':
			inDouble = true
			cur.WriteByte(c)
		case c == '\\' && i+1 < len(cmd):
			cur.WriteByte(c)
			i++
			cur.WriteByte(cmd[i])
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			i = writeSubshell(i)
		case c == '`':
			i = writeBacktick(i)
		case c == '\n' || c == ';':
			flush()
		case c == '&' && i+1 < len(cmd) && cmd[i+1] == '&':
			flush()
			i++ // skip second &
		case c == '|' && i+1 < len(cmd) && cmd[i+1] == '|':
			flush()
			i++ // skip second |
		default:
			cur.WriteByte(c)
		}
	}
	flush()

	return commands
}

// CollectAllCommands returns every command that would execute from a
// potentially compound, potentially nested shell command string.
//
// It splits on command separators (&&, ||, ;, \n) and recursively
// descends into $() and backtick subshells. Each level's commands are
// included in the result so that callers can verify every command
// matches a permission rule.
//
// Example: "echo $(git status && date)" returns:
//
//	["echo $(git status && date)", "git status", "date"]
func CollectAllCommands(cmd string) []string {
	var all []string

	for _, part := range SplitCommands(cmd) {
		all = append(all, part)
		for _, sub := range extractSubshells(part) {
			all = append(all, CollectAllCommands(sub)...)
		}
	}

	return all
}

// extractSubshells returns the contents of all $() and backtick command
// substitutions found in cmd outside of single quotes.
//
// Subshells inside double quotes are included since they expand there.
// Only single quotes suppress expansion.
func extractSubshells(cmd string) []string {
	var result []string
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		// Inside single quotes: everything is literal
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}

		// Inside double quotes: $() and backticks still expand
		if inDouble {
			if c == '\\' && i+1 < len(cmd) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
				continue
			}
			if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' {
				content, end := extractParenContent(cmd, i+2)
				if end >= 0 {
					result = append(result, content)
					i = end
				}
				continue
			}
			if c == '`' {
				content, end := extractBacktickContent(cmd, i+1)
				if end >= 0 {
					result = append(result, content)
					i = end
				}
				continue
			}
			continue
		}

		// Outside all quotes
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(cmd) {
				i++
			}
		case '$':
			if i+1 < len(cmd) && cmd[i+1] == '(' {
				content, end := extractParenContent(cmd, i+2)
				if end >= 0 {
					result = append(result, content)
					i = end
				}
			}
		case '`':
			content, end := extractBacktickContent(cmd, i+1)
			if end >= 0 {
				result = append(result, content)
				i = end
			}
		}
	}
	return result
}

// extractParenContent finds the matching ) for a $( that started at
// position start, handling nested parentheses and quotes.
// Returns the content between the parens and the index of the closing ).
func extractParenContent(cmd string, start int) (string, int) {
	depth := 1
	inSingle := false
	inDouble := false

	for i := start; i < len(cmd); i++ {
		c := cmd[i]

		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			if c == '\\' && i+1 < len(cmd) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue // parens inside double quotes don't affect depth
		}

		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(cmd) {
				i++
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return cmd[start:i], i
			}
		case '`':
			// Skip backtick content — parens inside don't affect our depth
			for j := i + 1; j < len(cmd); j++ {
				if cmd[j] == '\\' && j+1 < len(cmd) {
					j++
					continue
				}
				if cmd[j] == '`' {
					i = j
					break
				}
			}
		}
	}
	return "", -1 // unmatched
}

// extractBacktickContent finds the matching closing backtick starting
// from position start. Returns the content and the index of the closing `.
func extractBacktickContent(cmd string, start int) (string, int) {
	for i := start; i < len(cmd); i++ {
		if cmd[i] == '\\' && i+1 < len(cmd) {
			i++
			continue
		}
		if cmd[i] == '`' {
			return cmd[start:i], i
		}
	}
	return "", -1 // unmatched
}
