package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/lczyk/gitgum/internal/dirty"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
	"github.com/lczyk/gitgum/src/filetree"
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
	tracked, err := c.repo().DirtyTracked()
	if err != nil {
		return func() {}, err
	}
	return handleDirtyEntries(c, label, tracked)
}

// handleDirtyEntries is handleDirtyTree with the working-tree scan already
// done -- tracked is DirtyTracked's output. Callers that fetch the status
// concurrently with their other pre-picker reads (branch, switch) use this so
// the scan isn't serialised behind them; everyone else goes through
// handleDirtyTree.
func handleDirtyEntries(c *cmdIO, label string, tracked []git.Entry) (cleanup func(), err error) {
	noop := func() {}
	if len(tracked) == 0 {
		return noop, nil
	}

	// If the plan can't be read, the discard option is simply not offered
	// rather than shown with a count that might be wrong.
	plan, planErr := dirty.Scan(c.repo())
	discardOpts := dirty.Options{Tracked: true, Untracked: true}

	if planErr == nil {
		printDirty(c.out(), plan, discardOpts)
	} else {
		// No scan means no untracked half and no discard row, so what
		// triggered the prompt is all there is to show.
		items := statusItems(tracked, nil)
		fmt.Fprintf(c.out(), "Uncommitted changes (%d):\n", len(items))
		filetree.Tree(c.out(), items, treeOpts(maxDirtyPromptLines))
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

// maxDirtyPromptLines keeps a rebase touching hundreds of files from pushing
// the question itself off screen. The count in the header stays exact either
// way -- it is the number that decides whether you pick the discard row.
const maxDirtyPromptLines = 40

// printDirty lists the working tree ahead of the prompt: tracked changes and
// untracked files in one tree, each marked with the code it arrived with.
// Untracked files are here because discarding removes them, not because they
// blocked anything -- a distinction the discard row states.
func printDirty(out io.Writer, plan dirty.Plan, opts dirty.Options) {
	fmt.Fprintf(out, "Uncommitted changes (%d)%s\n", plan.Count(opts), planSummary(plan, opts))
	filetree.Tree(out, planItems(plan, opts), treeOpts(maxDirtyPromptLines))
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
