package git

import (
	"context"
	"fmt"
	"strings"
)

// Checkout switches the working tree to the given branch (quiet mode).
// Caller should provide the user-facing context; the returned error wraps
// stderr so it surfaces in the message chain.
func (r Repo) Checkout(branch string) error {
	_, stderr, err := r.runWrite(context.Background(), "checkout", "--quiet", branch)
	if err != nil {
		return fmt.Errorf("git checkout %s: %w: %s", branch, err, stderr)
	}
	return nil
}

// CheckoutNewBranch creates a new branch off startPoint and switches to it
// (`git checkout -b <branch> <startPoint>`).
func (r Repo) CheckoutNewBranch(branch, startPoint string) error {
	return r.checkoutNewBranch(branch, startPoint, false)
}

// CheckoutNewBranchNoTrack is CheckoutNewBranch with `--no-track`. Needed when
// startPoint is a remote-tracking ref: git's branch.autoSetupMerge default would
// silently set the new branch's upstream to it, so a later push would target
// someone else's branch instead of creating one.
func (r Repo) CheckoutNewBranchNoTrack(branch, startPoint string) error {
	return r.checkoutNewBranch(branch, startPoint, true)
}

func (r Repo) checkoutNewBranch(branch, startPoint string, noTrack bool) error {
	args := []string{"checkout", "-b", branch}
	if noTrack {
		args = append(args, "--no-track")
	}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	_, stderr, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git checkout -b %s: %w: %s", branch, err, stderr)
	}
	return nil
}

// BranchMerged reports whether branch is fully merged into HEAD -- the same
// question `git branch -d` answers by refusing. Asking it up front means the
// force-delete prompt can be raised alongside the other questions, rather than
// after an attempt that has to fail before it becomes informative.
func (r Repo) BranchMerged(branch string) bool {
	stdout, _, err := r.run("branch", "--merged", "HEAD", "--format=%(refname:short)")
	if err != nil {
		return false // unknown counts as unmerged: the cost is one extra prompt
	}
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.TrimSpace(line) == branch {
			return true
		}
	}
	return false
}

// ResetHard performs `git reset --hard <ref>`. Destructive: discards
// uncommitted changes; caller is responsible for asking the user first.
func (r Repo) ResetHard(ref string) error {
	_, stderr, err := r.runWrite(context.Background(), "reset", "--hard", ref)
	if err != nil {
		return fmt.Errorf("git reset --hard %s: %w: %s", ref, err, stderr)
	}
	return nil
}
