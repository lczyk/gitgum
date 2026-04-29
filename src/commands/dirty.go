package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/strutil"
)

// handleDirtyTree inspects the working tree. Untracked files are ignored.
// If tracked changes (staged or unstaged) exist, the user is asked whether
// to stash and continue. Returns stashed=true if a stash was created.
//
// label names the calling subcommand and appears in both the prompt and
// the stash message ("gitgum <label> auto-stash") so users can identify
// auto-stashes left behind by aborted runs.
func (c *cmdIO) handleDirtyTree(label string) (stashed bool, err error) {
	out, _, err := cmdrun.Run("git", "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	if out == "" {
		return false, nil
	}

	var dirty []string
	for _, line := range strutil.SplitLines(out) {
		if strings.HasPrefix(line, "?? ") {
			continue
		}
		dirty = append(dirty, line)
	}
	if len(dirty) == 0 {
		return false, nil
	}

	fmt.Fprintf(c.out(), "Uncommitted changes:\n%s\n", strings.Join(dirty, "\n"))
	prompt := fmt.Sprintf("Stash changes, run %s, then pop stash?", label)
	confirmed, err := c.sel().Confirm(prompt, false)
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, fmt.Errorf("aborted: working tree not clean")
	}
	stashMsg := fmt.Sprintf("gitgum %s auto-stash", label)
	if err := cmdrun.RunWithOutput("git", "stash", "push", "-m", stashMsg); err != nil {
		return false, fmt.Errorf("git stash push: %w", err)
	}
	return true, nil
}

// restoreStash pops the auto-stash. Uses --index to preserve the original
// staged-vs-unstaged split (including partial-hunk staging). Falls back to
// plain pop if --index conflicts with the new HEAD; the user is told to
// resolve manually if even that fails.
func (c *cmdIO) restoreStash() {
	if err := cmdrun.RunQuiet("git", "stash", "pop", "--index"); err == nil {
		return
	}
	if _, _, err := cmdrun.Run("git", "stash", "pop"); err != nil {
		fmt.Fprintf(c.err(), "warning: git stash pop failed: %v\n", err)
		fmt.Fprintln(c.err(), "your changes are still in the stash; run `git stash pop` manually")
		return
	}
	fmt.Fprintln(c.err(), "warning: could not restore exact index state; staged changes are now unstaged")
}
