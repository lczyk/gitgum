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

	// fetch once so producers do map lookups instead of N subprocess calls.
	checkedOut, err := git.CheckedOutBranches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting worktrees: %v\n", err)
		checkedOut = map[string]bool{}
	}

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

	go streamLocalBranches(ctx, queue, currentBranch, checkedOut)
	for _, remote := range remotes {
		go streamRemoteBranches(ctx, queue, remote, currentBranch, trackingRemote, checkedOut)
	}

	return &branches, &lock
}

func streamLocalBranches(ctx context.Context, queue chan<- branchEntry, currentBranch string, checkedOut map[string]bool) {
	locals, err := git.GetLocalBranches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting local branches: %v\n", err)
		return
	}

	for _, branch := range locals {
		if branch == currentBranch {
			continue
		}
		if checkedOut[branch] {
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

func streamRemoteBranches(ctx context.Context, queue chan<- branchEntry, remote, currentBranch, trackingRemote string, checkedOut map[string]bool) {
	branches, err := git.GetRemoteBranches(remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting remote branches for '%s': %v\n", remote, err)
		return
	}

	for _, branch := range branches {
		if remote == trackingRemote && branch == currentBranch {
			continue
		}
		if checkedOut[branch] {
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
