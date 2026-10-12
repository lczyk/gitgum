package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/ui"
)

// planRemoteSelection resolves a picked remote branch into the action that
// lands on it. Either way it ends on a local branch tracking the remote, then
// hands off to the shared pull flow (pullCurrent) -- the same "fetch + offer to
// integrate" every switch selection ends with. When no local branch exists yet
// it offers to create a tracking one; when one does, it switches and re-points
// tracking at this remote if needed.
//
// The offer is made here rather than inside the returned action because
// declining it cancels the switch, and a cancelled switch must not have cost
// the caller anything (see Execute: the dirty-tree answer is still unspent).
func (s *SwitchCommand) planRemoteSelection(remote, branch string) (func() error, error) {
	if !s.repo().BranchExists(branch) {
		confirmed, err := s.sel().Confirm(
			fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", branch), true)
		if err != nil {
			return nil, err
		}
		if !confirmed {
			fmt.Fprintln(s.err(), "Not creating a local tracking branch. Aborting switch.")
			return nil, ui.ErrCancelled
		}
		return func() error {
			if err := s.repo().CheckoutNewBranch(branch, remote+"/"+branch); err != nil {
				return fmt.Errorf("creating tracking branch: %w", err)
			}
			fmt.Fprintf(s.out(), "Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
				branch, remote, branch)
			return s.pullCurrent()
		}, nil
	}

	return func() error {
		if err := s.checkoutBranch(branch); err != nil {
			return err
		}
		trackingRemote, _ := s.repo().GetBranchTrackingRemote(branch)
		if trackingRemote == remote {
			fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", branch)
			return s.pullCurrent()
		}

		// The retarget is the one step here that is not the goal itself: it
		// only earns its keep if the pull that follows lands. A pull that
		// fails, is declined or is interrupted hands the branch back with the
		// upstream it had, so the switch never leaves tracking pointing at a
		// remote whose history the branch doesn't carry.
		intr := catchInterrupts()
		defer intr.stop()
		restore, err := s.retargetTracking(remote, branch)
		if err != nil {
			return err
		}
		fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", branch)
		if err := s.pullCurrent(); err != nil {
			return restore(intr.or(err))
		}
		return nil
	}, nil
}

// retargetTracking points branch's upstream at remote/branch and returns the
// undo: a function that puts the previous upstream back (or unsets it, if there
// was none) and hands through the error that prompted it, annotated when the
// restore itself fails.
//
// The previous upstream is restored from the raw branch.<name>.remote /
// .merge pair rather than via --set-upstream-to, which refuses an upstream
// whose remote-tracking ref is absent locally (a narrow fetch refspec) --
// exactly the config that was there a moment ago.
func (s *SwitchCommand) retargetTracking(remote, branch string) (restore func(error) error, err error) {
	fmt.Fprintf(s.out(), "Local branch '%s' is not tracking remote branch '%s/%s'.\n", branch, remote, branch)

	prevRemote, err := s.repo().BranchConfigGet(branch, "remote")
	if err != nil {
		return nil, err
	}
	prevMerge, err := s.repo().BranchConfigGet(branch, "merge")
	if err != nil {
		return nil, err
	}

	if _, stderr, err := s.repo().RunWrite("branch", "--set-upstream-to="+remote+"/"+branch, branch); err != nil {
		return nil, fmt.Errorf("setting tracking reference: %w: %s", err, stderr)
	}
	fmt.Fprintf(s.out(), "Set tracking reference for local branch '%s' to remote branch '%s/%s'.\n",
		branch, remote, branch)

	return func(cause error) error {
		var (
			rerr error
			was  string
		)
		if prevRemote == "" || prevMerge == "" {
			was = "no upstream"
			_, stderr, err := s.repo().RunWrite("branch", "--unset-upstream", branch)
			if err != nil {
				rerr = fmt.Errorf("%w: %s", err, stderr)
			}
		} else {
			was = "'" + upstreamShort(prevRemote, prevMerge) + "'"
			if err := s.repo().BranchConfigSet(branch, "remote", prevRemote); err != nil {
				rerr = err
			} else if err := s.repo().BranchConfigSet(branch, "merge", prevMerge); err != nil {
				rerr = err
			}
		}
		if rerr != nil {
			return fmt.Errorf("%w; restoring the tracking reference of '%s' to %s failed too (%v)",
				cause, branch, was, rerr)
		}
		fmt.Fprintf(s.err(), "Restored the tracking reference of '%s' to %s.\n", branch, was)
		return cause
	}, nil
}

// upstreamShort renders a branch.<name>.remote / .merge pair the way git's
// %(upstream:short) would: "<remote>/<branch>", or just "<branch>" for the
// local remote ".".
func upstreamShort(remote, merge string) string {
	branch := strings.TrimPrefix(merge, "refs/heads/")
	if remote == "." {
		return branch
	}
	return remote + "/" + branch
}
