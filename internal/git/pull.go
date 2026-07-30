package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// PullMode selects how fetched upstream commits are integrated into the
// current branch. It maps onto git's three integration strategies.
type PullMode int

const (
	PullFFOnly PullMode = iota // git merge --ff-only: refuse a merge commit
	PullRebase                 // git rebase: replay local commits onto upstream
	PullMerge                  // git merge: merge commit when the branches diverged
)

// Integrate applies upstream into the current branch using the chosen mode.
// The caller is expected to have Fetch'd first: Integrate works against the
// local remote-tracking ref (e.g. "origin/main") and does not touch the
// network. Progress and hook output are streamed live. A shallow clone stays
// shallow -- nothing here deepens history.
func (r Repo) Integrate(mode PullMode, upstream string) error {
	var args []string
	switch mode {
	case PullFFOnly:
		// --no-stat: the caller renders its own compact-summary of what landed,
		// so git's plain diffstat would just be a duplicate.
		args = []string{"merge", "--no-stat", "--ff-only", upstream}
	case PullRebase:
		args = []string{"rebase", upstream}
	case PullMerge:
		args = []string{"merge", "--no-stat", upstream}
	default:
		return fmt.Errorf("unknown pull mode %d", mode)
	}
	if err := r.runWriteStreaming(context.Background(), args...); err != nil {
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}

// WouldRebaseConflict reports whether replaying `branch`'s commits onto
// `upstream` (a pull --rebase) would hit textual conflicts, without touching
// the working tree, index, or HEAD. It uses `git merge-tree --write-tree`,
// which performs the three-way merge in memory and exits 1 on conflict, 0 on a
// clean result.
//
// That tests a merge of the two tips rather than a commit-by-commit replay, so
// it's a close proxy for rebase cleanliness rather than an exact oracle: the
// net changes integrated are the same, so a clean merge-tree almost always
// means a clean rebase and a conflicting one almost always means a conflicting
// rebase. Callers use it to decide whether to offer a rebase, not to promise
// one.
//
// --write-tree needs git >= 2.38; on older git the option is unknown and this
// returns an error, which the caller should surface as "couldn't check" rather
// than guess.
func (r Repo) WouldRebaseConflict(upstream, branch string) (conflict bool, err error) {
	_, stderr, err := r.run("merge-tree", "--write-tree", upstream, branch)
	if err == nil {
		return false, nil
	}
	// merge-tree exits 1 specifically for "merged, but with conflicts"; any
	// other non-zero exit (bad ref, unknown option on old git) is a real error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git merge-tree: %w: %s", err, strings.TrimSpace(stderr))
}

func (mode PullMode) String() string {
	switch mode {
	case PullFFOnly:
		return "fast-forward only"
	case PullRebase:
		return "rebase"
	case PullMerge:
		return "merge"
	default:
		return fmt.Sprintf("PullMode(%d)", int(mode))
	}
}
