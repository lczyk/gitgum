package commands

import (
	"fmt"

	"github.com/lczyk/gitgum/internal/ui"
)

// handleRemoteSelection handles switching when the user picked a remote branch.
// If a local branch with the same name exists, offers to align tracking + reset.
// Otherwise offers to create a tracking branch.
func (s *SwitchCommand) handleRemoteSelection(remote, branch string) error {
	fmt.Fprintf(s.out(), "Fetching '%s/%s'...\n", remote, branch)
	if err := s.repo().Fetch(remote, branch); err != nil {
		return fmt.Errorf("fetching '%s/%s': %w", remote, branch, err)
	}

	if !s.repo().BranchExists(branch) {
		return s.createTrackingBranch(remote, branch)
	}

	fmt.Fprintf(s.out(), "Branch '%s/%s' already has a local counterpart.\n", remote, branch)

	trackingRemote, _ := s.repo().GetBranchTrackingRemote(branch)
	if trackingRemote != "" {
		fmt.Fprintf(s.out(), "Tracking reference for local branch '%s': '%s'\n", branch, trackingRemote)
	}

	if trackingRemote != remote {
		if err := s.retargetTracking(remote, branch); err != nil {
			return err
		}
	}

	if err := s.checkoutBranch(branch); err != nil {
		return err
	}

	return s.alignWithRemote(remote, branch)
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

func (s *SwitchCommand) alignWithRemote(remote, branch string) error {
	localCommit, err := s.repo().GetCommitHash(branch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}

	remoteRef := remote + "/" + branch
	remoteCommit, err := s.repo().GetCommitHash(remoteRef)
	if err != nil {
		return fmt.Errorf("getting remote commit for '%s': %w", remoteRef, err)
	}

	if localCommit == remoteCommit {
		fmt.Fprintf(s.out(), "Local branch '%s' is up to date with remote branch '%s/%s'.\n", branch, remote, branch)
		fmt.Fprintf(s.out(), "Switched to branch '%s' tracking remote branch '%s/%s'.\n", branch, remote, branch)
		return nil
	}

	confirmed, err := s.sel().Confirm(fmt.Sprintf("Local branch '%s' is not up to date with remote branch '%s/%s'. Reset the local branch to the remote branch?",
		branch, remote, branch), false)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(s.err(), "Not resetting local branch.")
		fmt.Fprintf(s.out(), "Switched to branch '%s' (diverged from '%s/%s').\n", branch, remote, branch)
		return nil
	}
	if err := s.repo().ResetHard(remoteRef); err != nil {
		return fmt.Errorf("resetting local branch: %w", err)
	}
	fmt.Fprintf(s.out(), "Switched to branch '%s', reset to remote branch '%s/%s'.\n", branch, remote, branch)
	return nil
}

// warnIfRemoteUnreachable checks the tracking remote of a just-checked-out
// local branch and prints a warning if the branch is missing there or the
// remote can't be reached. Best-effort: never fails the switch, since the
// checkout already succeeded.
func (s *SwitchCommand) warnIfRemoteUnreachable(branch string) {
	remote, err := s.repo().GetBranchTrackingRemote(branch)
	if err != nil || remote == "" {
		return
	}
	exists, reachable := s.repo().RemoteBranchReachability(remote, branch)
	switch {
	case exists:
		return
	case reachable:
		fmt.Fprintf(s.err(), "Warning: branch '%s' no longer exists on remote '%s' (deleted upstream?). Local tracking info is stale.\n", branch, remote)
	default:
		fmt.Fprintf(s.err(), "Warning: could not reach remote '%s' to verify branch '%s' (network/auth issue?). Local tracking info may be stale.\n", remote, branch)
	}
}

func (s *SwitchCommand) createTrackingBranch(remote, branch string) error {
	confirmed, err := s.sel().Confirm(fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", branch), true)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(s.err(), "Not creating a local tracking branch. Aborting switch.")
		return ui.ErrCancelled
	}
	if err := s.repo().CheckoutNewBranch(branch, remote+"/"+branch); err != nil {
		return fmt.Errorf("creating tracking branch: %w", err)
	}
	fmt.Fprintf(s.out(), "Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
		branch, remote, branch)
	return nil
}
