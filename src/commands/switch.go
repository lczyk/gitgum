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

type SwitchCommand struct{}

func (s *SwitchCommand) checkoutBranch(branch string) error {
	_, stderr, err := internal.RunCommand("git", "checkout", "--quiet", branch)
	if err != nil {
		return fmt.Errorf("could not switch to branch '%s': %s", branch, stderr)
	}
	return nil
}

func (s *SwitchCommand) Execute() error {
	// validation: ensure we're in a git repo with clean working tree
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	currentBranch, err := internal.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	trackingRemote, err := internal.GetBranchTrackingRemote(currentBranch)
	if err != nil {
		return fmt.Errorf("getting tracking remote: %w", err)
	}

	branchDisplay := currentBranch
	if trackingRemote != "" {
		branchDisplay = fmt.Sprintf("(%s/)%s", trackingRemote, currentBranch)
	}
	fmt.Println("Current branch is:", branchDisplay)

	dirty, err := internal.IsGitDirty(".")
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "You have local changes that would be overwritten by switching branches. Please commit or stash them before switching.")
		return fmt.Errorf("local changes would be overwritten")
	}

	// stream branches from all remotes and local in parallel for faster picker feedback

	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	var (
		branchLock   sync.Mutex
		branches     []string
		seenBranches = make(map[string]struct{})
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// stream branches from git in background, deduplicating and throttling for UX
	type branchEntry struct {
		display  string
		dedupKey string
	}

	streamQueue := make(chan branchEntry, 1000)
	go func() {
		for {
			select {
			case entry := <-streamQueue:
				time.Sleep(streamDelay)
				branchLock.Lock()
				if _, seen := seenBranches[entry.dedupKey]; !seen {
					seenBranches[entry.dedupKey] = struct{}{}
					branches = append(branches, entry.display)
				}
				branchLock.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

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
				tr, err := internal.GetBranchTrackingRemote(branch)
				if err != nil {
					tr = ""
				}
				if tr != "" {
					streamQueue <- branchEntry{
						display:  "local/remote: " + branch,
						dedupKey: "remote:" + tr + "/" + branch,
					}
				} else {
					streamQueue <- branchEntry{
						display:  "local: " + branch,
						dedupKey: "local:" + branch,
					}
				}
			}
		}
	}()

	// stream remote branches in parallel
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
						streamQueue <- branchEntry{
							display:  "remote: " + r + "/" + branch,
							dedupKey: "remote:" + r + "/" + branch,
						}
					}
				}
			}
		}()
	}

	// user selection: let user pick from live-updating branch list
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

	parts := strings.SplitN(selected, ": ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid selection: %s", selected)
	}

	typ := parts[0]
	name := parts[1]

	// handle branch selection: different logic for local vs remote branches
	switch typ {
	case "local":
		branch := name
		if err := s.checkoutBranch(branch); err != nil {
			return err
		}

		fmt.Printf("Switched to branch '%s'.\n", branch)
	case "remote":
		remoteBranchParts := strings.SplitN(name, "/", 2)
		if len(remoteBranchParts) != 2 {
			return fmt.Errorf("invalid remote branch format: %s", name)
		}

		remote := remoteBranchParts[0]
		branch := remoteBranchParts[1]

		if internal.BranchExists(branch) {
			fmt.Printf("Branch '%s' is already tracked locally as '%s'.\n", branch, branch)

			trackingRemote, err := internal.GetBranchTrackingRemote(branch)
			if err != nil {
				trackingRemote = ""
			}

			if trackingRemote != "" {
				fmt.Printf("Tracking reference for local branch '%s': '%s'\n", branch, trackingRemote)
			}

			// offer to update tracking reference if it points to a different remote
			if trackingRemote != remote {
				fmt.Printf("Local branch '%s' is not tracking remote branch '%s/%s'.\n",
					branch, remote, branch)

				confirmed, err := internal.FzfConfirm(fmt.Sprintf("Set '%s/%s' as the tracking reference for local branch '%s'?",
					remote, branch, branch), false)
				if err != nil {
					return err
				}
				if confirmed {
					if err := internal.RunCommandQuiet("git", "branch", "--set-upstream-to="+remote+"/"+branch, branch); err != nil {
						return fmt.Errorf("setting tracking reference: %w", err)
					}

					fmt.Printf("Set tracking reference for local branch '%s' to remote branch '%s/%s'.\n",
						branch, remote, branch)
				} else {
					fmt.Fprintln(os.Stderr, "Not setting tracking reference. Aborting switch.")
					return fmt.Errorf("user cancelled")
				}
			}

			if err := s.checkoutBranch(branch); err != nil {
				return err
			}

			localCommit, err := internal.GetCommitHash(branch)
			if err != nil {
				return fmt.Errorf("getting local commit: %w", err)
			}

			remoteRef := remote + "/" + branch
			remoteCommit, err := internal.GetCommitHash(remoteRef)
			if err != nil {
				return fmt.Errorf("getting remote commit for '%s': %w", remoteRef, err)
			}

			if localCommit == remoteCommit {
				fmt.Printf("Local branch '%s' is up to date with remote branch '%s/%s'.\n",
					branch, remote, branch)
				fmt.Printf("Switched to branch '%s' tracking remote branch '%s/%s'.\n",
					branch, remote, branch)
				return nil
			}

			confirmed, err := internal.FzfConfirm(fmt.Sprintf("Local branch '%s' is not up to date with remote branch '%s/%s'. Reset the local branch to the remote branch?",
				branch, remote, branch), false)
			if err != nil {
				return err
			}
			if confirmed {
				if err := internal.RunCommandQuiet("git", "reset", "--hard", remoteRef); err != nil {
					return fmt.Errorf("resetting local branch: %w", err)
				}

				fmt.Printf("Reset local branch '%s' to remote branch '%s/%s'.\n",
					branch, remote, branch)
			} else {
				fmt.Fprintln(os.Stderr, "Not resetting local branch.")
			}

			return nil
		}

		// branch doesn't exist locally — offer to create tracking branch
		confirmed, err := internal.FzfConfirm(fmt.Sprintf("Branch '%s' is not tracked locally. Create a local tracking branch?", branch), true)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(os.Stderr, "Not creating a local tracking branch. Aborting switch.")
			return fmt.Errorf("user cancelled")
		}

		if err := internal.RunCommandQuiet("git", "checkout", "-b", branch, remote+"/"+branch); err != nil {
			return fmt.Errorf("creating tracking branch: %w", err)
		}

		fmt.Printf("Created and switched to local branch '%s' tracking remote branch '%s/%s'.\n",
			branch, remote, branch)
	default:
		return fmt.Errorf("unknown branch type: %s", typ)
	}

	return nil
}

const streamDelay = 3 * time.Millisecond
