package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/ui"
)

type PushCommand struct {
	cmdIO
}

func (p *PushCommand) Execute(args []string) error {
	if err := p.repo().CheckInRepo(); err != nil {
		return err
	}

	remoteBranch, err := p.repo().GetCurrentBranchUpstream()
	if err != nil {
		return err
	}
	if remoteBranch != "" {
		fmt.Fprintf(p.out(), "Current branch already has a remote tracking branch: %s\n", remoteBranch)
		confirmed, err := p.sel().Confirm("Do you want to push to the remote tracking branch?", true)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return fmt.Errorf("confirming push to upstream: %w", err)
		}
		if !confirmed {
			return nil
		}
		if err := p.repo().Push(); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Fprintf(p.out(), "Pushed to remote tracking branch '%s'.\n", remoteBranch)
		return nil
	}

	currentBranch, err := p.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	remotes, err := p.repo().GetRemotes()
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
		remote, err := p.sel().Select(fmt.Sprintf("Push '%s' to", currentBranch), remotes, query...)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return fmt.Errorf("selecting remote: %w", err)
		}
		selectedRemote = remote
	}

	expectedRemoteBranchName := selectedRemote + "/" + currentBranch

	remoteBranchExists, err := p.repo().RemoteBranchExists(selectedRemote, currentBranch)
	if err != nil {
		return fmt.Errorf("checking remote branch: %w", err)
	}

	if !remoteBranchExists {
		confirmed, err := p.sel().Confirm(fmt.Sprintf("No remote branch '%s' found. Do you want to create it?",
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

		if _, stderr, err := p.repo().RunWriteStream("push", "-u", selectedRemote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %w: %s", err, strings.TrimSpace(stderr))
		}
		fmt.Fprintf(p.out(), "Created and set tracking reference for '%s' to '%s'.\n",
			currentBranch, expectedRemoteBranchName)
		return nil
	}

	localCommit, err := p.repo().GetCommitHash(currentBranch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}

	remoteCommit, err := p.repo().GetCommitHash(expectedRemoteBranchName)
	if err != nil {
		return fmt.Errorf("could not find remote branch '%s': %w", expectedRemoteBranchName, err)
	}

	if localCommit == remoteCommit {
		fmt.Fprintf(p.out(), "No changes to push. Local branch '%s' is up to date with remote branch '%s'.\n",
			currentBranch, expectedRemoteBranchName)
		// set upstream since we're targeting this remote
		if _, _, err := p.repo().RunWrite("branch", "--set-upstream-to="+expectedRemoteBranchName, currentBranch); err != nil {
			return fmt.Errorf("failed to set upstream: %w", err)
		}
		fmt.Fprintf(p.out(), "Updated upstream to '%s'.\n", expectedRemoteBranchName)
		return nil
	}

	confirmed, err := p.sel().Confirm(fmt.Sprintf("Remote branch '%s' already exists. Do you want to push to it?",
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

	if _, stderr, err := p.repo().RunWriteStream("push", selectedRemote, currentBranch); err != nil {
		return fmt.Errorf("failed to push: %w: %s", err, strings.TrimSpace(stderr))
	}
	fmt.Fprintf(p.out(), "Pushed to remote branch '%s'.\n", expectedRemoteBranchName)
	return nil
}
