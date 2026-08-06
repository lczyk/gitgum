package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// DirtyTracked lists tracked changes, staged or unstaged. Untracked files are
// left out: callers use this to decide whether the working tree is dirty in a
// way that blocks stashing, switching or releasing, and an untracked file
// blocks none of those.
//
// Entries keep both status characters, so a caller can tell " M" (unstaged)
// from "M " (staged) from "MM" (partial-hunk staging).
//
// The scan asks git not to look for untracked files at all rather than
// dropping them afterwards: on a big tree that walk is most of what a status
// costs, and this is the read every dirty-tree prompt is gated on.
func (r Repo) DirtyTracked() ([]Entry, error) {
	_, entries, err := r.Status(ScanOpts{Untracked: UntrackedNone})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// inProgressMarkers maps a path under the git dir to the operation it means is
// underway, in the order git's own prompt script checks them: a paused rebase
// looks like several of these at once, and the more specific answer is the
// useful one. sequencer/todo catches a multi-step cherry-pick or revert whose
// per-commit marker was already cleared by an intermediate commit.
var inProgressMarkers = []struct{ path, operation string }{
	{"rebase-merge", "an interactive rebase"},
	{"rebase-apply", "a rebase"},
	{"MERGE_HEAD", "a merge"},
	{"CHERRY_PICK_HEAD", "a cherry-pick"},
	{"REVERT_HEAD", "a revert"},
	{"BISECT_LOG", "a bisect"},
	{"sequencer/todo", "a cherry-pick or revert sequence"},
}

// InProgress reports whether the repo is part-way through an operation that
// owns the working tree -- a merge, rebase, cherry-pick, revert or bisect --
// and names it if so.
//
// This matters before anything discards changes: `git reset --hard` in the
// middle of a merge does not merely drop edits, it clears MERGE_HEAD and ends
// the merge, so a command that means "throw away my changes" would quietly do
// something much larger.
//
// Unmerged index entries are checked too, since a conflict can outlive its
// marker file once the operation is partly resolved.
func (r Repo) InProgress() (operation string, yes bool) {
	gitDir, _, err := r.run("rev-parse", "--absolute-git-dir")
	if err == nil && gitDir != "" {
		for _, marker := range inProgressMarkers {
			if _, err := os.Stat(filepath.Join(gitDir, marker.path)); err == nil {
				return marker.operation, true
			}
		}
	}
	if unmerged, _, err := r.run("diff", "--name-only", "--diff-filter=U"); err == nil && unmerged != "" {
		return "an unresolved conflict", true
	}
	return "", false
}

// stashHooksOff suppresses user pre-stash / post-checkout hooks for the
// duration of a stash op. gg uses stash internally only (release auto-
// stash, switch_stream bookkeeping); firing user hooks on plumbing they
// didn't initiate is a footgun. Shared, so build argv with slices.Concat:
// append would write into its backing array once it has spare capacity.
var stashHooksOff = []string{"-c", "core.hooksPath=/dev/null"}

// StashPush stashes tracked changes (staged + unstaged) under the given
// message. Untracked files are not included.
func (r Repo) StashPush(message string) error {
	args := slices.Concat(stashHooksOff, []string{"stash", "push", "-m", message})
	_, _, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git stash push: %w", err)
	}
	return nil
}

// StashPopIndex pops the most recent stash with --index, restoring the
// exact staged-vs-unstaged split (including partial-hunk staging). On
// conflict, git leaves the stash entry in place; the caller should treat
// that as a manual-resolution situation rather than retrying.
func (r Repo) StashPopIndex() error {
	args := slices.Concat(stashHooksOff, []string{"stash", "pop", "--index"})
	_, _, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git stash pop --index: %w", err)
	}
	return nil
}
