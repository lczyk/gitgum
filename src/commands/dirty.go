package commands

import (
	"errors"
	"fmt"
	"strings"
)

// errDirtyTreeAborted is returned by handleDirtyTree when the user declines
// the stash prompt. Callers can use errors.Is to distinguish a clean abort
// from a real failure.
var errDirtyTreeAborted = errors.New("aborted: working tree not clean")

// handleDirtyTree inspects the working tree. Untracked files are ignored.
// If tracked changes (staged or unstaged) exist, the user is asked whether
// to stash and continue.
//
// Returns a cleanup function that callers should always defer. cleanup is
// a no-op if nothing was stashed; otherwise it pops the stash with --index
// to preserve the original staged-vs-unstaged split (including partial-
// hunk staging). On pop conflict, cleanup leaves the stash in place and
// warns — callers must not retry, since git's partial state would compound.
//
// label names the calling subcommand and appears in both the prompt and
// the stash message ("gitgum <label> auto-stash") so users can identify
// auto-stashes left behind.
func handleDirtyTree(c *cmdIO, label string) (cleanup func(), err error) {
	noop := func() {}

	dirty, err := c.repo().DirtyTrackedLines()
	if err != nil {
		return noop, err
	}
	if len(dirty) == 0 {
		return noop, nil
	}

	fmt.Fprintf(c.out(), "Uncommitted changes:\n%s\n", strings.Join(dirty, "\n"))
	prompt := fmt.Sprintf("Stash changes, run %s, then pop stash?", label)
	confirmed, err := c.sel().Confirm(prompt, false)
	if err != nil {
		return noop, err
	}
	if !confirmed {
		return noop, errDirtyTreeAborted
	}

	stashMsg := fmt.Sprintf("gitgum %s auto-stash", label)
	if err := c.repo().StashPush(stashMsg); err != nil {
		return noop, err
	}

	return func() {
		if err := c.repo().StashPopIndex(); err != nil {
			fmt.Fprintf(c.err(), "warning: %v\n", err)
			fmt.Fprintf(c.err(), "your changes are still in the stash (%q); resolve and run `git stash pop --index` manually\n", stashMsg)
		}
	}, nil
}
