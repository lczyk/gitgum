package commands

import (
	"fmt"

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
		if trackingRemote != remote {
			if err := s.retargetTracking(remote, branch); err != nil {
				return err
			}
		}
		fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", branch)
		return s.pullCurrent()
	}, nil
}

func (s *SwitchCommand) retargetTracking(remote, branch string) error {
	fmt.Fprintf(s.out(), "Local branch '%s' is not tracking remote branch '%s/%s'.\n", branch, remote, branch)

	if _, stderr, err := s.repo().RunWrite("branch", "--set-upstream-to="+remote+"/"+branch, branch); err != nil {
		return fmt.Errorf("setting tracking reference: %w: %s", err, stderr)
	}
	fmt.Fprintf(s.out(), "Set tracking reference for local branch '%s' to remote branch '%s/%s'.\n",
		branch, remote, branch)
	return nil
}
