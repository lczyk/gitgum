package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ktr0731/go-fuzzyfinder"
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

	// Get remotes for parallel fetching
	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("error getting remotes: %v", err)
	}

	var (
		branchLock   sync.Mutex
		branches     []string
		seenBranches = make(map[string]struct{})
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamQueue := make(chan string, 1000)
	go func() {
		for {
			select {
			case branch := <-streamQueue:
				time.Sleep(streamDelay)
				branchLock.Lock()
				if _, seen := seenBranches[branch]; !seen {
					seenBranches[branch] = struct{}{}
					branches = append(branches, branch)
				}
				branchLock.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Stream local branches
	go func() {
		locals, err := internal.GetLocalBranches()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting local branches: %v\n", err)
			return
		}

		for _, branch := range locals {
			if branch == currentBranch {
				continue
			}
			isCheckedOut, _, err := internal.IsWorktreeCheckedOut(branch)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error checking worktree status for local branch '%s': %v\n", branch, err)
				continue
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
				streamQueue <- prefix + ": " + branch
			}
		}
	}()

	// Query remotes in parallel and stream their branches
	for _, remote := range remotes {
		r := remote
		go func() {
			branches, err := internal.GetRemoteBranches(r)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting remote branches for '%s': %v\n", r, err)
				return
			}

			for _, branch := range branches {
				if !(r == trackingRemote && branch == currentBranch) {
					isCheckedOut, _, err := internal.IsWorktreeCheckedOut(branch)
					if err != nil {
						fmt.Fprintf(os.Stderr, "error checking worktree status for remote branch '%s': %v\n", branch, err)
						continue
					}
					if !isCheckedOut {
						streamQueue <- "remote: " + r + "/" + branch
					}
				}
			}
		}()
	}

	prompt := "Select a branch to switch to"
	selectedIdx, err := fuzzyfinder.Find(
		&branches,
		func(i int) string { return branches[i] },
		fuzzyfinder.WithPromptString(prompt+": "),
		fuzzyfinder.WithMatcher(func(query, item string) bool {
			words := strings.Fields(query)
			for _, word := range words {
				if !strings.Contains(strings.ToLower(item), strings.ToLower(word)) {
					return false
				}
			}
			return true
		}),
		fuzzyfinder.WithHotReloadLock(&branchLock),
		fuzzyfinder.WithContext(ctx),
	)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "No branch selected. Aborting switch.")
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return internal.ErrFzfCancelled
		}
		return err
	}

	branchLock.Lock()
	if selectedIdx < 0 || selectedIdx >= len(branches) {
		branchLock.Unlock()
		return fmt.Errorf("invalid branch selection index: %d", selectedIdx)
	}
	selected := branches[selectedIdx]
	branchLock.Unlock()

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

const streamDelay = 3 * time.Millisecond

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
	_, stderr, err := internal.RunCommand("git", "checkout", "--quiet", localBranch)
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
