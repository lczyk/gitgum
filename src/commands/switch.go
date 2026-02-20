package commands

import (
	"fmt"
	"os"
	"strings"

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

	// Get all branches
	branches, err := getAllBranches(currentBranch, trackingRemote)
	if err != nil {
		return fmt.Errorf("error getting branches: %v", err)
	}

	if len(branches) == 0 {
		fmt.Fprintln(os.Stderr, "No branches available to switch to. Aborting switch.")
		return fmt.Errorf("no branches available")
	}

	// Let user select a branch
	selected, err := internal.FzfSelect("Select a branch to switch to", branches)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No branch selected. Aborting switch.")
		return err
	}

	// Parse selection
	parts := strings.SplitN(selected, ": ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid selection: %s", selected)
	}

	typ := parts[0]
	name := parts[1]

	if typ == "local" || typ == "local/remote" {
		branch := name
		// Switch to the branch
		_, stderr, err := internal.RunCommand("git", "checkout", "--quiet", branch)
		if err != nil {
			return fmt.Errorf("could not switch to branch '%s': %s", branch, stderr)
		}

		fmt.Printf("Switched to branch '%s'.\n", branch)
	} else if typ == "remote" {
		// Parse remote/branch
		remoteBranchParts := strings.SplitN(name, "/", 2)
		if len(remoteBranchParts) != 2 {
			return fmt.Errorf("invalid remote branch format: %s", name)
		}

		remote := remoteBranchParts[0]
		branch := remoteBranchParts[1]

		// Handle remote branch switching
		if internal.BranchExists(branch) {
			return handleExistingLocalBranch(branch, remote)
		}

		// Branch doesn't exist locally, ask to create tracking branch
		confirmed, err := internal.FzfConfirm(fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", branch), true)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(os.Stderr, "Not creating a local tracking branch. Aborting switch.")
			return fmt.Errorf("user cancelled")
		}

		// Create and checkout the tracking branch
		if err := internal.RunCommandQuiet("git", "checkout", "-b", branch, remote+"/"+branch); err != nil {
			return fmt.Errorf("could not create tracking branch: %v", err)
		}

		fmt.Printf("Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
			branch, remote, branch)
	} else {
		return fmt.Errorf("unknown branch type: %s", typ)
	}

	return nil
}

// getAllBranches collects all available local and remote branches for selection
func getAllBranches(currentBranch, trackingRemote string) ([]string, error) {
	var allBranches []string

	// Get local branches
	locals, err := internal.GetLocalBranches()
	if err != nil {
		return nil, fmt.Errorf("error getting local branches: %v", err)
	}

	for _, branch := range locals {
		if branch != currentBranch {
			isCheckedOut, _, err := internal.IsWorktreeCheckedOut(branch)
			if err != nil {
				return nil, fmt.Errorf("error checking worktree status for local branch '%s': %v", branch, err)
			}
			if !isCheckedOut {
				trackingRemote, err := internal.GetBranchTrackingRemote(branch)
				if err != nil {
					trackingRemote = ""
				}
				prefix := "local"
				if trackingRemote != "" {
					prefix = "local/remote"
				}
				allBranches = append(allBranches, prefix+": "+branch)
			}
		}
	}

	// Get remote branches
	remotes, err := internal.GetRemotes()
	if err != nil {
		return nil, fmt.Errorf("error getting remotes: %v", err)
	}

	for _, remote := range remotes {
		remoteBranches, err := internal.GetRemoteBranches(remote)
		if err != nil {
			return nil, fmt.Errorf("error getting remote branches for '%s': %v", remote, err)
		}

		for _, branch := range remoteBranches {
			if !(remote == trackingRemote && branch == currentBranch) {
				isCheckedOut, _, err := internal.IsWorktreeCheckedOut(branch)
				if err != nil {
					return nil, fmt.Errorf("error checking worktree status for remote branch '%s': %v", branch, err)
				}
				if !isCheckedOut {
					allBranches = append(allBranches, "remote: "+remote+"/"+branch)
				}
			}
		}
	}

	return allBranches, nil
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
