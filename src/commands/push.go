package commands

import (
	"errors"
	"fmt"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type PushCommand struct {
	cmdIO
}

func (p *PushCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	remoteBranch, err := git.GetCurrentBranchUpstream()
	if err != nil {
		return err
	}
	if remoteBranch != "" {
		fmt.Fprintf(p.out(), "Current branch already has a remote tracking branch: %s\n", remoteBranch)
		confirmed, err := ui.Confirm("Do you want to push to the remote tracking branch?", true)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return fmt.Errorf("confirming push to upstream: %w", err)
		}
		if !confirmed {
			return nil
		}
		if err := cmdrun.RunWithOutput("git", "push"); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Fprintf(p.out(), "Pushed to remote tracking branch '%s'.\n", remoteBranch)
		return nil
	}

	currentBranch, err := git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	remotes, err := git.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	if len(remotes) == 0 {
		return fmt.Errorf("no remotes")
	}

	var selectedRemote string
	if len(args) > 0 {
		for _, r := range remotes {
			if r == args[0] {
				selectedRemote = r
				break
			}
		}
	}
	if selectedRemote == "" {
		var query []string
		if len(args) > 0 {
			query = args[:1]
		}
		remote, err := ui.Select(fmt.Sprintf("Push '%s' to", currentBranch), remotes, query...)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return fmt.Errorf("selecting remote: %w", err)
		}
		selectedRemote = remote
	}

	expectedRemoteBranchName := selectedRemote + "/" + currentBranch

	remoteBranchExists, err := git.RemoteBranchExists(selectedRemote, currentBranch)
	if err != nil {
		return fmt.Errorf("checking remote branch: %w", err)
	}

	if !remoteBranchExists {
		confirmed, err := ui.Confirm(fmt.Sprintf("No remote branch '%s' found. Do you want to create it?",
			expectedRemoteBranchName), false)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return fmt.Errorf("confirming create remote branch: %w", err)
		}
		if !confirmed {
			return nil
		}

		if err := cmdrun.RunWithOutput("git", "push", "-u", selectedRemote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Fprintf(p.out(), "Created and set tracking reference for '%s' to '%s'.\n",
			currentBranch, expectedRemoteBranchName)
		return nil
	}

	localCommit, err := git.GetCommitHash(currentBranch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}

	remoteCommit, err := git.GetCommitHash(expectedRemoteBranchName)
	if err != nil {
		return fmt.Errorf("could not find remote branch '%s': %w", expectedRemoteBranchName, err)
	}

	if localCommit == remoteCommit {
		fmt.Fprintf(p.out(), "No changes to push. Local branch '%s' is up to date with remote branch '%s'.\n",
			currentBranch, expectedRemoteBranchName)
		// set upstream since we're targeting this remote
		if err := cmdrun.RunQuiet("git", "branch", "--set-upstream-to="+expectedRemoteBranchName, currentBranch); err != nil {
			return fmt.Errorf("failed to set upstream: %w", err)
		}
		fmt.Fprintf(p.out(), "Updated upstream to '%s'.\n", expectedRemoteBranchName)
		return nil
	}

	confirmed, err := ui.Confirm(fmt.Sprintf("Remote branch '%s' already exists. Do you want to push to it?",
		expectedRemoteBranchName), true)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return nil
		}
		return fmt.Errorf("confirming push to remote: %w", err)
	}
	if !confirmed {
		return nil
	}

	if err := cmdrun.RunWithOutput("git", "push", selectedRemote, currentBranch); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	fmt.Fprintf(p.out(), "Pushed to remote branch '%s'.\n", expectedRemoteBranchName)
	return nil
}
