package commands

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

const streamDelay = 3 * time.Millisecond

type branchEntry struct {
	display  string
	dedupKey string
}

// streamBranches collects local and remote branches concurrently, deduplicating
// and writing into a SliceSource the picker consumes. Caller cancels ctx
// when the consumer (fuzzyfinder) is done; cancellation also stops producers.
// errOut receives non-fatal diagnostic messages from the producers.
//
// The returned SliceSource supports both Add (used by the producers below)
// and RemoveFunc (left available for future hooks that drop branches as the
// user deletes them).
func streamBranches(ctx context.Context, errOut io.Writer, currentBranch, trackingRemote string, remotes []string) *fuzzyfinder.SliceSource {
	src := fuzzyfinder.NewSliceSource()
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	// Fetch checkout state once so producers can do map lookups instead of
	// N subprocess calls.
	checkedOut, err := git.CheckedOutBranches()
	if err != nil {
		fmt.Fprintf(errOut, "error getting worktrees: %v\n", err)
		checkedOut = map[string]bool{}
	}

	queue := make(chan branchEntry, 1000)
	go func() {
		for {
			select {
			case entry := <-queue:
				time.Sleep(streamDelay)
				seenMu.Lock()
				if _, ok := seen[entry.dedupKey]; ok {
					seenMu.Unlock()
					continue
				}
				seen[entry.dedupKey] = struct{}{}
				seenMu.Unlock()
				src.Add(entry.display)
			case <-ctx.Done():
				return
			}
		}
	}()

	go streamLocalBranches(ctx, errOut, queue, currentBranch, checkedOut)
	for _, remote := range remotes {
		go streamRemoteBranches(ctx, errOut, queue, remote, currentBranch, trackingRemote, checkedOut)
	}

	// Known limitation: branches deleted in another shell while the picker
	// is open stay in the list until the user closes and reopens it. The
	// underlying SliceSource supports RemoveFunc, but periodically polling
	// git from inside the picker isn't worth the complexity for a stale
	// entry that fails loudly on selection (`git checkout` errors). If this
	// becomes a real annoyance, hook a rescanner here.
	return src
}

func streamLocalBranches(ctx context.Context, errOut io.Writer, queue chan<- branchEntry, currentBranch string, checkedOut map[string]bool) {
	locals, err := git.GetLocalBranches()
	if err != nil {
		fmt.Fprintf(errOut, "error getting local branches: %v\n", err)
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

func streamRemoteBranches(ctx context.Context, errOut io.Writer, queue chan<- branchEntry, remote, currentBranch, trackingRemote string, checkedOut map[string]bool) {
	branches, err := git.GetRemoteBranches(remote)
	if err != nil {
		fmt.Fprintf(errOut, "error getting remote branches for '%s': %v\n", remote, err)
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
