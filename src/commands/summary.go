package commands

import "github.com/lczyk/gitgum/internal/git"

// compactSummary runs `git diff --compact-summary` with the extra args (paths,
// ranges, --cached, ...), coloured per the environment. It's the shared
// diffstat renderer used by diff, pull, push and clone, so it lives here rather
// than inside any one command's file.
//
// Repo.Run TrimSpaces stdout, eating the leading space git emits on every row;
// it is restored so the first row aligns with the rest. Empty diff -> "" (no
// leading space).
func compactSummary(r git.Repo, extra ...string) (string, error) {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	args := append([]string{"diff", "--compact-summary", colorFlag}, extra...)
	out, _, err := r.Run(args...)
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", nil
	}
	return " " + out, nil
}
