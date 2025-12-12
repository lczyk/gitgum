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

	// Define the two modes
	modeLocal := "Switch to an existing local branch"
	modeRemote := "Switch to an existing remote branch and create a local tracking branch"

	modes := []string{modeLocal, modeRemote}

	// Get current branch
	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("error getting current branch: %v", err)
	}

	// Get tracking remote
	trackingRemote, err := internal.GetBranchTrackingRemote(currentBranch)
	if err != nil {
		return fmt.Errorf("error getting tracking remote: %v", err)
	}

	// Show current branch
	fmt.Println("Current branch is:", func() string {
		if trackingRemote != "" {
			return fmt.Sprintf("(%s/)%s", trackingRemote, currentBranch)
		}
		return currentBranch
	}())

	// Check if there are any local changes that would be overwritten
	dirty, err := internal.IsGitDirty(".")
	if err != nil {
		return fmt.Errorf("error checking git status: %v", err)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "You have local changes that would be overwritten by switching branches. Please commit or stash them before switching.")
		return fmt.Errorf("local changes would be overwritten")
	}

	// Ask user what they want to do
	selected, err := internal.FzfSelect("What do you want to do?", modes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No action selected. Aborting switch.")
		return err
	}

	// Execute the selected mode
	switch selected {
	case modeLocal:
		return switchLocal(currentBranch, trackingRemote)
	case modeRemote:
		return switchRemote(currentBranch, trackingRemote)
	default:
		return fmt.Errorf("unknown option: %s", selected)
	}
}

// switchLocal switches to an existing local branch
func switchLocal(currentBranch, trackingRemote string) error {
	// Get all local branches
	branches, err := internal.GetLocalBranches()
	if err != nil {
		return fmt.Errorf("error getting local branches: %v", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No local branches found. Aborting switch.")
		return fmt.Errorf("no local branches")
	}

	// Filter out the current branch
	var filteredBranches []string
	for _, branch := range branches {
		if branch != currentBranch {
			filteredBranches = append(filteredBranches, branch)
		}
	}

	if len(filteredBranches) == 0 {
		fmt.Fprintln(os.Stderr, "No other local branches found. Aborting switch.")
		return fmt.Errorf("no other local branches")
	}

	// Let user select a branch
	branch, err := internal.FzfSelect("Select a branch to switch to", filteredBranches)
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
	_, stderr, err := internal.RunCommand("git", "checkout", "--quiet", branch);
	if err != nil {
		return fmt.Errorf("could not switch to branch '%s': %s", branch, stderr)
	}

	fmt.Printf("Switched to branch '%s'.\n", branch)
	return nil
}

// switchRemote creates a local tracking branch from a remote branch
func switchRemote(currentBranch, trackingRemote string) error {
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

	// Filter out the current branch's tracking remote
	if remote == trackingRemote {
		var filteredBranches []string
		for _, branch := range remoteBranches {
			if branch != currentBranch {
				filteredBranches = append(filteredBranches, branch)
			}
		}
		remoteBranches = filteredBranches
		
		if len(remoteBranches) == 0 {
			fmt.Fprintln(os.Stderr, "No other remote branches found. Aborting switch.")
			return fmt.Errorf("no other remote branches")
		}
	}

	// Filter out branches checked out in other worktrees
	var availableBranches []string
	for _, branch := range remoteBranches {
		isCheckedOut, _, err := internal.IsWorktreeCheckedOut(branch)
		if err != nil {
			return fmt.Errorf("error checking worktree status: %v", err)
		}
		if !isCheckedOut {
			availableBranches = append(availableBranches, branch)
		}
	}

	if len(availableBranches) == 0 {
		fmt.Fprintln(os.Stderr, "No available remote branches found (all are checked out in other worktrees). Aborting switch.")
		return fmt.Errorf("no available remote branches")
	}

	remoteBranches = availableBranches
	
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
	_, stderr, err := internal.RunCommand("git", "checkout", "--quiet", localBranch);
	if err != nil {
		return fmt.Errorf("could not switch to branch '%s': %s", localBranch, stderr)
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
