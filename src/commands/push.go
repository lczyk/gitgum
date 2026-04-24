package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/lczyk/gitgum/src/internal"
)

type PushCommand struct{}

func (p *PushCommand) Execute(args []string) error {
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	remoteBranch, err := internal.GetCurrentBranchUpstream()
	if err != nil {
		return err
	}
	if remoteBranch != "" {
		fmt.Printf("Current branch already has a remote tracking branch: %s\n", remoteBranch)
		confirmed, err := internal.FzfConfirm("Do you want to push to the remote tracking branch?", true)
		if err != nil {
			if errors.Is(err, internal.ErrFzfCancelled) {
				return nil
			}
			return err
		}
		if confirmed {
			if err := internal.RunCommandWithOutput("git", "push"); err != nil {
				return fmt.Errorf("failed to push: %w", err)
			}
			fmt.Printf("Pushed to remote tracking branch '%s'.\n", remoteBranch)
			return nil
		}
		return nil
	}

	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(os.Stderr, "No remotes found. Aborting push.")
		return fmt.Errorf("no remotes")
	}

	var selectedRemote string
	if len(args) > 0 {
		providedRemote := args[0]
		for _, r := range remotes {
			if r == providedRemote {
				selectedRemote = r
				break
			}
		}
		if selectedRemote == "" {
			remote, err := internal.FzfSelect(fmt.Sprintf("Push '%s' to", currentBranch), remotes, providedRemote)
			if err != nil {
				if errors.Is(err, internal.ErrFzfCancelled) {
					return nil
				}
				return fmt.Errorf("selecting remote: %w", err)
			}
			selectedRemote = remote
		}
	} else {
		remote, err := internal.FzfSelect(fmt.Sprintf("Push '%s' to", currentBranch), remotes)
		if err != nil {
			if errors.Is(err, internal.ErrFzfCancelled) {
				return nil
			}
			return fmt.Errorf("selecting remote: %w", err)
		}
		selectedRemote = remote
	}

	expectedRemoteBranchName := selectedRemote + "/" + currentBranch

	remoteBranchExists, err := internal.RemoteBranchExists(selectedRemote, currentBranch)
	if err != nil {
		return fmt.Errorf("checking remote branch: %w", err)
	}

	if remoteBranchExists {
		localCommit, err := internal.GetCommitHash(currentBranch)
		if err != nil {
			return fmt.Errorf("getting local commit: %w", err)
		}

		remoteCommit, err := internal.GetCommitHash(expectedRemoteBranchName)
		if err != nil {
			return fmt.Errorf("could not find remote branch '%s': %w", expectedRemoteBranchName, err)
		}

		if localCommit == remoteCommit {
			fmt.Printf("No changes to push. Local branch '%s' is up to date with remote branch '%s'.\n",
				currentBranch, expectedRemoteBranchName)
			// set upstream since we're targeting this remote
			if err := internal.RunCommandQuiet("git", "branch", "--set-upstream-to="+expectedRemoteBranchName, currentBranch); err != nil {
				return fmt.Errorf("failed to set upstream: %w", err)
			}
			fmt.Printf("Updated upstream to '%s'.\n", expectedRemoteBranchName)
			return nil
		}

		confirmed, err := internal.FzfConfirm(fmt.Sprintf("Remote branch '%s' already exists. Do you want to push to it?",
			expectedRemoteBranchName), true)
		if err != nil {
			if errors.Is(err, internal.ErrFzfCancelled) {
				return nil
			}
			return err
		}
		if !confirmed {
			return nil
		}

		if err := internal.RunCommandWithOutput("git", "push", selectedRemote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
	} else {
		confirmed, err := internal.FzfConfirm(fmt.Sprintf("No remote branch '%s' found. Do you want to create it?",
			expectedRemoteBranchName), false)
		if err != nil {
			if errors.Is(err, internal.ErrFzfCancelled) {
				return nil
			}
			return err
		}
		if !confirmed {
			return nil
		}

		if err := internal.RunCommandWithOutput("git", "push", "-u", selectedRemote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}

		fmt.Printf("Created and set tracking reference for '%s' to '%s'.\n",
			currentBranch, expectedRemoteBranchName)
	}

	return nil
}
