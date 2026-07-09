package normalize

import "strings"

// CommandNormalizer strips global flags that appear between a command
// name and its subcommand, so that prefix-based rule matching works
// regardless of flag position.
//
// For example, git allows global flags before the subcommand:
//
//	git -C /foo --no-pager log --oneline
//
// The normalizer strips -C /foo and --no-pager, producing:
//
//	git log --oneline
type CommandNormalizer struct {
	Command      string
	FlagsWithArg []string // flags that consume the next token: "-C", "-c"
	FlagsNoArg   []string // standalone flags: "--no-pager", "--bare"
	FlagPrefixes []string // flags using = or joined syntax: "--git-dir="
}

var gitNormalizer = CommandNormalizer{
	Command:      "git",
	FlagsWithArg: []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--super-prefix", "--config-env"},
	FlagsNoArg:   []string{"--no-pager", "--bare", "--no-replace-objects", "--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs", "--no-optional-locks", "--no-lazy-fetch", "-P", "-p", "--paginate"},
	FlagPrefixes: []string{"--git-dir=", "--work-tree=", "--namespace=", "--super-prefix=", "--config-env="},
}

var normalizers = map[string]*CommandNormalizer{
	"git": &gitNormalizer,
}

// Command normalizes a command given its pre-parsed arguments from the
// shell AST. args[0] is the command name, args[1:] are the arguments.
// Global flags are stripped and the normalized command is returned as a
// joined string. Returns the input unchanged if no normalizer matches.
func Command(args []string) string {
	if len(args) == 0 {
		return ""
	}
	n, ok := normalizers[args[0]]
	if !ok {
		return strings.Join(args, " ")
	}
	return n.normalize(args[1:])
}

func (n *CommandNormalizer) normalize(args []string) string {
	var rest []string

	for i := 0; i < len(args); i++ {
		tok := args[i]

		if n.isPrefixFlag(tok) {
			continue
		}

		if n.isFlagWithArg(tok) {
			i++ // skip the argument
			continue
		}

		if n.isFlagNoArg(tok) {
			continue
		}

		// Not a known global flag — this is the subcommand
		rest = append(rest, args[i:]...)
		break
	}

	if len(rest) == 0 {
		return n.Command
	}
	return n.Command + " " + strings.Join(rest, " ")
}

func (n *CommandNormalizer) isPrefixFlag(tok string) bool {
	for _, p := range n.FlagPrefixes {
		if strings.HasPrefix(tok, p) && len(tok) > len(p) {
			return true
		}
	}
	return false
}

func (n *CommandNormalizer) isFlagWithArg(tok string) bool {
	for _, f := range n.FlagsWithArg {
		if tok == f {
			return true
		}
	}
	return false
}

func (n *CommandNormalizer) isFlagNoArg(tok string) bool {
	for _, f := range n.FlagsNoArg {
		if tok == f {
			return true
		}
	}
	return false
}
