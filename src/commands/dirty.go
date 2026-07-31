package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/lczyk/gitgum/internal/dirty"
	"github.com/lczyk/gitgum/internal/ui"
)

// The three ways past a dirty tree. Abort leads so Enter still declines and
// the irreversible choice sits furthest from the cursor -- every destructive
// prompt in gg defaults to the answer that destroys nothing.
const (
	dirtyAbort   = "abort"
	dirtyStash   = "stash"
	dirtyDiscard = "discard"
)

// handleDirtyTree inspects the working tree. Untracked files do not trigger the
// prompt -- they do not block the operations that ask -- but discarding does
// remove them, so they are listed alongside the tracked changes and counted in
// the discard row.
//
// Returns a cleanup function that callers should always defer. cleanup is a
// no-op unless something was stashed; otherwise it pops the stash with --index
// to preserve the original staged-vs-unstaged split (including partial-hunk
// staging). On pop conflict, cleanup leaves the stash in place and warns --
// callers must not retry, since git's partial state would compound.
//
// label names the calling subcommand and appears in both the prompt and the
// stash message ("gitgum <label> auto-stash") so users can identify auto-stashes
// left behind.
func handleDirtyTree(c *cmdIO, label string) (cleanup func(), err error) {
	dirtyLines, err := c.repo().DirtyTrackedLines()
	if err != nil {
		return func() {}, err
	}
	return handleDirtyLines(c, label, dirtyLines)
}

// handleDirtyLines is handleDirtyTree with the working-tree scan already done --
// dirty is DirtyTrackedLines' output. Callers that fetch the status concurrently
// with their other pre-picker reads (branch, switch) use this so the scan isn't
// serialised behind them; everyone else goes through handleDirtyTree.
func handleDirtyLines(c *cmdIO, label string, dirtyLines []string) (cleanup func(), err error) {
	noop := func() {}
	if len(dirtyLines) == 0 {
		return noop, nil
	}

	fmt.Fprintf(c.out(), "Uncommitted changes:\n%s\n", strings.Join(dirtyLines, "\n"))

	// The plan describes the discard option honestly. If it cannot be read,
	// the option is simply not offered rather than offered with a number that
	// might be wrong.
	plan, planErr := dirty.Scan(c.repo())
	discardOpts := dirty.Options{Tracked: true, Untracked: true}

	// Untracked files do not block the operation, so they are not why this
	// prompt appeared -- but discarding removes them, and a count without the
	// names is what made gg clean dangerous. List them when there are any.
	if planErr == nil {
		printUntracked(c.out(), plan.Untracked)
	}

	options := []string{dirtyAbort, dirtyStashOption(label)}
	if planErr == nil {
		options = append(options, discardLabel(plan, discardOpts))
	}

	selected, err := c.sel().Select("Uncommitted changes -- what now?", options)
	if err != nil {
		return noop, err
	}
	switch {
	case selected == options[0]:
		// choosing abort and pressing escape are the same decision
		return noop, ui.ErrCancelled
	case len(options) > 2 && selected == options[2]:
		if err := plan.Discard(c.repo(), discardOpts); err != nil {
			return noop, err
		}
		fmt.Fprintf(c.out(), "Discarded %d file(s).\n", plan.Count(discardOpts))
		return noop, nil
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

// maxUntrackedShown bounds the untracked list. The count stays exact -- it is
// the number that decides whether you pick the discard row.
const maxUntrackedShown = 10

// printUntracked lists the files discarding would remove on top of the tracked
// changes already shown. Silent when there are none, so the common case reads
// exactly as it did before.
func printUntracked(out io.Writer, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(out, "Untracked files (%d, discarding removes these too):\n", len(paths))
	for i, p := range paths {
		if i >= maxUntrackedShown {
			fmt.Fprintf(out, "  ... and %d more\n", len(paths)-maxUntrackedShown)
			break
		}
		fmt.Fprintf(out, "  %s\n", p)
	}
}

// dirtyStashOption is the stash row, shared so tests name the row rather than
// restating its wording.
func dirtyStashOption(label string) string {
	return fmt.Sprintf("stash changes, run %s, then pop stash", label)
}

// discardLabel states what the row will destroy. The counts belong in the row
// rather than above it: discarding twelve files and discarding forty thousand
// are different decisions, and the row is what you are reading when you decide.
func discardLabel(plan dirty.Plan, opts dirty.Options) string {
	parts := []string{}
	if n := len(plan.Tracked); n > 0 {
		parts = append(parts, fmt.Sprintf("%d change(s)", n))
	}
	if n := len(plan.Untracked); n > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked file(s)", n))
	}
	return fmt.Sprintf("%s %s -- cannot be undone", dirtyDiscard, strings.Join(parts, " and "))
}
