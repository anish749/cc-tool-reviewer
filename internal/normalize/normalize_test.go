package normalize

import "testing"

func TestCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"strip -C", []string{"git", "-C", "/foo", "log", "--oneline"}, "git log --oneline"},
		{"strip --no-pager", []string{"git", "--no-pager", "diff", "HEAD"}, "git diff HEAD"},
		{"strip -c with value", []string{"git", "-c", "core.pager=cat", "status"}, "git status"},
		{"multiple flags", []string{"git", "-C", "/foo", "-c", "x=y", "--no-pager", "log"}, "git log"},
		{"no global flags", []string{"git", "log", "--oneline"}, "git log --oneline"},
		{"bare git", []string{"git"}, "git"},
		{"prefix flag --git-dir=", []string{"git", "--git-dir=/foo", "status"}, "git status"},
		{"space flag --work-tree", []string{"git", "--work-tree", "/foo", "status"}, "git status"},
		{"--bare standalone", []string{"git", "--bare", "init"}, "git init"},
		{"-P standalone", []string{"git", "-P", "log"}, "git log"},
		{"no normalizer", []string{"echo", "hello"}, "echo hello"},
		{"only flags no subcommand", []string{"git", "-C", "/foo", "--no-pager"}, "git"},
		{"preserves subcommand args", []string{"git", "-C", "/foo", "log", "--all", "--since=2026-01-01", "--oneline"}, "git log --all --since=2026-01-01 --oneline"},
		{"empty args", []string{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Command(tc.args)
			if got != tc.want {
				t.Errorf("Command(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
