package git

import (
	"context"
	"fmt"
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

// ResetHard performs `git reset --hard <ref>`. Destructive: discards
// uncommitted changes; caller is responsible for asking the user first.
func (r Repo) ResetHard(ref string) error {
	_, stderr, err := r.runWrite(context.Background(), "reset", "--hard", ref)
	if err != nil {
		return fmt.Errorf("git reset --hard %s: %w: %s", ref, err, stderr)
	}
	return nil
}
