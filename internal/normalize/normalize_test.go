package normalize

import "testing"

func TestCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strip -C", "git -C /foo log --oneline", "git log --oneline"},
		{"strip --no-pager", "git --no-pager diff HEAD", "git diff HEAD"},
		{"strip -c with =", "git -c core.pager=cat status", "git status"},
		{"strip -c with space", "git -c core.pager status", "git status"},
		{"multiple flags", "git -C /foo -c x=y --no-pager log", "git log"},
		{"no global flags", "git log --oneline", "git log --oneline"},
		{"bare git", "git", "git"},
		{"prefix flag --git-dir=", "git --git-dir=/foo status", "git status"},
		{"space flag --work-tree", "git --work-tree /foo status", "git status"},
		{"--bare standalone", "git --bare init", "git init"},
		{"-P standalone", "git -P log", "git log"},
		{"no normalizer", "echo hello", "echo hello"},
		{"only flags no subcommand", "git -C /foo --no-pager", "git"},
		{"preserves subcommand args", "git -C /foo log --all --since=2026-01-01 --oneline", "git log --all --since=2026-01-01 --oneline"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Command(tc.input)
			if got != tc.want {
				t.Errorf("Command(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
