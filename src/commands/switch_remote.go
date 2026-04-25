package commands

import (
	"fmt"
	"os"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

// handleRemoteSelection handles switching when the user picked a remote branch.
// If a local branch with the same name exists, offers to align tracking + reset.
// Otherwise offers to create a tracking branch.
func (s *SwitchCommand) handleRemoteSelection(remote, branch string) error {
	if !git.BranchExists(branch) {
		return s.createTrackingBranch(remote, branch)
	}

	fmt.Printf("Branch '%s' is already tracked locally as '%s'.\n", branch, branch)

	trackingRemote, err := git.GetBranchTrackingRemote(branch)
	if err != nil {
		trackingRemote = ""
	}
	if trackingRemote != "" {
		fmt.Printf("Tracking reference for local branch '%s': '%s'\n", branch, trackingRemote)
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
	fmt.Printf("Local branch '%s' is not tracking remote branch '%s/%s'.\n", branch, remote, branch)

	confirmed, err := ui.Confirm(fmt.Sprintf("Set '%s/%s' as the tracking reference for local branch '%s'?",
		remote, branch, branch), false)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Not setting tracking reference. Aborting switch.")
		return fmt.Errorf("user cancelled")
	}
	if err := cmdrun.RunQuiet("git", "branch", "--set-upstream-to="+remote+"/"+branch, branch); err != nil {
		return fmt.Errorf("setting tracking reference: %w", err)
	}
	fmt.Printf("Set tracking reference for local branch '%s' to remote branch '%s/%s'.\n",
		branch, remote, branch)
	return nil
}

func (s *SwitchCommand) alignWithRemote(remote, branch string) error {
	localCommit, err := git.GetCommitHash(branch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}

	remoteRef := remote + "/" + branch
	remoteCommit, err := git.GetCommitHash(remoteRef)
	if err != nil {
		return fmt.Errorf("getting remote commit for '%s': %w", remoteRef, err)
	}

	if localCommit == remoteCommit {
		fmt.Printf("Local branch '%s' is up to date with remote branch '%s/%s'.\n", branch, remote, branch)
		fmt.Printf("Switched to branch '%s' tracking remote branch '%s/%s'.\n", branch, remote, branch)
		return nil
	}

	confirmed, err := ui.Confirm(fmt.Sprintf("Local branch '%s' is not up to date with remote branch '%s/%s'. Reset the local branch to the remote branch?",
		branch, remote, branch), false)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Not resetting local branch.")
		return nil
	}
	if err := cmdrun.RunQuiet("git", "reset", "--hard", remoteRef); err != nil {
		return fmt.Errorf("resetting local branch: %w", err)
	}
	fmt.Printf("Reset local branch '%s' to remote branch '%s/%s'.\n", branch, remote, branch)
	return nil
}

func (s *SwitchCommand) createTrackingBranch(remote, branch string) error {
	confirmed, err := ui.Confirm(fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", branch), true)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Not creating a local tracking branch. Aborting switch.")
		return fmt.Errorf("user cancelled")
	}
	if err := cmdrun.RunQuiet("git", "checkout", "-b", branch, remote+"/"+branch); err != nil {
		return fmt.Errorf("creating tracking branch: %w", err)
	}
	fmt.Printf("Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
		branch, remote, branch)
	return nil
}
