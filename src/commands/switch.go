package commands

import (
	"fmt"
	"os"

	"github.com/lczyk/gitgum/src/internal"
)

// SwitchCommand handles interactive branch switching
type SwitchCommand struct{}

// Execute runs the switch command
func (s *SwitchCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Define the three modes
	modeLocal := "Switch to an existing local branch"
	modeRemote := "Switch to an existing remote branch and create a local tracking branch"
	modeNew := "Create a new branch (local)"

	modes := []string{modeLocal, modeRemote, modeNew}

	// Ask user what they want to do
	selected, err := internal.FzfSelect("What do you want to do?", modes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No action selected. Aborting switch.")
		return err
	}

	// Execute the selected mode
	switch selected {
	case modeLocal:
		return switchLocal()
	case modeRemote:
		return switchRemote()
	case modeNew:
		return switchNew()
	default:
		return fmt.Errorf("unknown option: %s", selected)
	}
}

// switchLocal switches to an existing local branch
func switchLocal() error {
	// Get all local branches
	branches, err := internal.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("error getting local branches: %v", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No local branches found. Aborting switch.")
		return fmt.Errorf("no local branches")
	}

	// Let user select a branch
	branch, err := internal.FzfSelect("Select a branch to switch to", branches)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No branch selected. Aborting switch.")
		return err
	}

	// Check if branch is checked out in another worktree
	isCheckedOut, worktreePath, err := internal.IsWorktreeCheckedOut(branch)
	if err != nil {
		return fmt.Errorf("error checking worktree status: %v", err)
	}

	if isCheckedOut {
		return fmt.Errorf("branch '%s' is already checked out in another worktree: %s", branch, worktreePath)
	}

	// Switch to the branch
	if err := internal.RunCommandQuiet("git", "checkout", "--quiet", branch); err != nil {
		return fmt.Errorf("could not switch to branch '%s': %v", branch, err)
	}

	fmt.Printf("Switched to branch '%s'.\n", branch)
	return nil
}

// switchRemote creates a local tracking branch from a remote branch
func switchRemote() error {
	// Get list of remotes
	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("error getting remotes: %v", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(os.Stderr, "No remotes found. Aborting switch.")
		return fmt.Errorf("no remotes")
	}

	// Select a remote
	remote, err := internal.FzfSelect("Select a remote", remotes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No remote selected. Aborting switch.")
		return err
	}

	// Get remote branches for this remote
	remoteBranches, err := internal.GetRemoteBranches(remote)
	if err != nil {
		return fmt.Errorf("error getting remote branches: %v", err)
	}

	if len(remoteBranches) == 0 {
		return fmt.Errorf("no remote branches found for remote '%s'", remote)
	}

	// Select a remote branch
	remoteBranch, err := internal.FzfSelect("Select a remote branch to switch to", remoteBranches)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No remote branch selected. Aborting switch.")
		return err
	}

	// Check if branch already exists locally
	if internal.BranchExists(remoteBranch) {
		return handleExistingLocalBranch(remoteBranch, remote)
	}

	// Branch doesn't exist locally, ask to create tracking branch
	confirmed, err := internal.FzfConfirm(fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", remoteBranch), true)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "Not creating a local tracking branch. Aborting switch.")
		return fmt.Errorf("user cancelled")
	}

	// Create and checkout the tracking branch
	if err := internal.RunCommandQuiet("git", "checkout", "-b", remoteBranch, remote+"/"+remoteBranch); err != nil {
		return fmt.Errorf("could not create tracking branch: %v", err)
	}

	fmt.Printf("Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
		remoteBranch, remote, remoteBranch)

	return nil
}

// handleExistingLocalBranch handles the case where a local branch already exists
func handleExistingLocalBranch(localBranch, remote string) error {
	fmt.Printf("Branch '%s' is already tracked locally as '%s'.\n", localBranch, localBranch)

	// Get current tracking remote
	trackingRemote, err := internal.GetBranchTrackingRemote(localBranch)
	if err != nil {
		trackingRemote = ""
	}

	if trackingRemote != "" {
		fmt.Printf("Tracking reference for local branch '%s': '%s'\n", localBranch, trackingRemote)
	}

	// Check if tracking remote matches
	if trackingRemote != remote {
		fmt.Printf("Local branch '%s' is not tracking remote branch '%s/%s'.\n",
			localBranch, remote, localBranch)

		confirmed, err := internal.FzfConfirm(fmt.Sprintf("Set '%s/%s' as the tracking reference for local branch '%s'?",
			remote, localBranch, localBranch), false)
		if err != nil {
			return err
		}
		if confirmed {
			if err := internal.RunCommandQuiet("git", "branch", "--set-upstream-to="+remote+"/"+localBranch, localBranch); err != nil {
				return fmt.Errorf("could not set tracking reference: %v", err)
			}

			fmt.Printf("Set tracking reference for local branch '%s' to remote branch '%s/%s'.\n",
				localBranch, remote, localBranch)
		} else {
			fmt.Fprintln(os.Stderr, "Not setting tracking reference. Aborting switch.")
			return fmt.Errorf("user cancelled")
		}
	}

	// Switch to the local branch
	if err := internal.RunCommandQuiet("git", "checkout", "--quiet", localBranch); err != nil {
		return fmt.Errorf("could not switch to local branch '%s': %v", localBranch, err)
	}

	// Check if local branch is up to date with remote
	localCommit, err := internal.GetCommitHash(localBranch)
	if err != nil {
		return fmt.Errorf("could not get local commit: %v", err)
	}

	remoteRef := remote + "/" + localBranch
	remoteCommit, err := internal.GetCommitHash(remoteRef)
	if err != nil {
		return fmt.Errorf("could not find remote branch '%s': %v", remoteRef, err)
	}

	if localCommit == remoteCommit {
		fmt.Printf("Local branch '%s' is up to date with remote branch '%s/%s'.\n",
			localBranch, remote, localBranch)
		fmt.Printf("Switched to branch '%s' tracking remote branch '%s/%s'.\n",
			localBranch, remote, localBranch)
		return nil
	}

	// Branch is not up to date
	confirmed, err := internal.FzfConfirm(fmt.Sprintf("Local branch '%s' is not up to date with remote branch '%s/%s'. Reset the local branch to the remote branch?",
		localBranch, remote, localBranch), false)
	if err != nil {
		return err
	}
	if confirmed {
		if err := internal.RunCommandQuiet("git", "reset", "--hard", remoteRef); err != nil {
			return fmt.Errorf("could not reset local branch: %v", err)
		}

		fmt.Printf("Reset local branch '%s' to remote branch '%s/%s'.\n",
			localBranch, remote, localBranch)
	} else {
		fmt.Fprintln(os.Stderr, "Not resetting local branch.")
	}

	return nil
}

// switchNew creates a new local branch
func switchNew() error {
	fmt.Fprintln(os.Stderr, "Error: This feature is not implemented yet.")
	return fmt.Errorf("not implemented")
}
