package commands

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lczyk/gitgum/internal/git"
)

const streamDelay = 3 * time.Millisecond

type branchEntry struct {
	display  string
	dedupKey string
}

// streamBranches collects local and remote branches concurrently, deduplicating
// and writing into a slice protected by the returned lock. Caller cancels ctx
// when the consumer (fuzzyfinder) is done; cancellation also stops producers.
func streamBranches(ctx context.Context, currentBranch, trackingRemote string, remotes []string) (*[]string, *sync.Mutex) {
	var (
		lock     sync.Mutex
		branches []string
		seen     = make(map[string]struct{})
	)

	queue := make(chan branchEntry, 1000)
	go func() {
		for {
			select {
			case entry := <-queue:
				time.Sleep(streamDelay)
				lock.Lock()
				if _, ok := seen[entry.dedupKey]; !ok {
					seen[entry.dedupKey] = struct{}{}
					branches = append(branches, entry.display)
				}
				lock.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	go streamLocalBranches(ctx, queue, currentBranch)
	for _, remote := range remotes {
		go streamRemoteBranches(ctx, queue, remote, currentBranch, trackingRemote)
	}

	return &branches, &lock
}

func streamLocalBranches(ctx context.Context, queue chan<- branchEntry, currentBranch string) {
	locals, err := git.GetLocalBranches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting local branches: %v\n", err)
		return
	}

	for _, branch := range locals {
		if branch == currentBranch {
			continue
		}
		isCheckedOut, _, err := git.IsWorktreeCheckedOut(branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error checking worktree status for local branch '%s': %v\n", branch, err)
			continue
		}
		if isCheckedOut {
			continue
		}
		tr, err := git.GetBranchTrackingRemote(branch)
		if err != nil {
			tr = ""
		}
		var entry branchEntry
		if tr != "" {
			entry = branchEntry{
				display:  "local/remote: " + branch,
				dedupKey: "remote:" + tr + "/" + branch,
			}
		} else {
			entry = branchEntry{
				display:  "local: " + branch,
				dedupKey: "local:" + branch,
			}
		}
		select {
		case queue <- entry:
		case <-ctx.Done():
			return
		}
	}
}

func streamRemoteBranches(ctx context.Context, queue chan<- branchEntry, remote, currentBranch, trackingRemote string) {
	branches, err := git.GetRemoteBranches(remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting remote branches for '%s': %v\n", remote, err)
		return
	}

	for _, branch := range branches {
		if remote == trackingRemote && branch == currentBranch {
			continue
		}
		isCheckedOut, _, err := git.IsWorktreeCheckedOut(branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error checking worktree status for remote branch '%s': %v\n", branch, err)
			continue
		}
		if isCheckedOut {
			continue
		}
		select {
		case queue <- branchEntry{
			display:  "remote: " + remote + "/" + branch,
			dedupKey: "remote:" + remote + "/" + branch,
		}:
		case <-ctx.Done():
			return
		}
	}
}
