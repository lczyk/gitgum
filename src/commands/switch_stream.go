package commands

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lczyk/gitgum/internal/git"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

const streamDelay = 3 * time.Millisecond

// checkedOutMarker tags a branch already checked out in another worktree. The
// switch flow always ends in `git checkout <name>` / reset, which git rejects
// for a branch checked out elsewhere, so these entries are shown but made
// unselectable (see switch.go). The emitter and the unselectable predicate both
// key on this string -- keep them in sync via this const.
const checkedOutMarker = " (checked out in "

// checkedOutSuffix renders the display suffix naming the blocking worktree.
func checkedOutSuffix(worktreePath string) string {
	return checkedOutMarker + filepath.Base(worktreePath) + " worktree)"
}

// isCheckedOutElsewhere is the picker's Unselectable predicate: an entry
// carrying the checked-out marker can't be switched to (see checkedOutMarker).
func isCheckedOutElsewhere(item string) bool {
	return strings.Contains(item, checkedOutMarker)
}

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
func streamBranches(ctx context.Context, r git.Repo, errOut io.Writer, currentBranch, trackingRemote string, remotes []string) *ff.SliceSource {
	src := ff.NewSliceSource()
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	// Fetch checkout state once so producers can do map lookups instead of
	// N subprocess calls.
	checkedOut, err := r.CheckedOutBranches()
	if err != nil {
		fmt.Fprintf(errOut, "error getting worktrees: %v\n", err)
		checkedOut = map[string]string{}
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

	go streamLocalBranches(ctx, r, errOut, queue, currentBranch, checkedOut)
	for _, remote := range remotes {
		go streamRemoteBranches(ctx, r, errOut, queue, remote, currentBranch, trackingRemote, checkedOut)
	}

	// Known limitation: branches deleted in another shell while the picker
	// is open stay in the list until the user closes and reopens it. The
	// underlying SliceSource supports RemoveFunc, but periodically polling
	// git from inside the picker isn't worth the complexity for a stale
	// entry that fails loudly on selection (`git checkout` errors). If this
	// becomes a real annoyance, hook a rescan goroutine here.
	return src
}

func streamLocalBranches(ctx context.Context, r git.Repo, errOut io.Writer, queue chan<- branchEntry, currentBranch string, checkedOut map[string]string) {
	locals, err := r.GetLocalBranches()
	if err != nil {
		fmt.Fprintf(errOut, "error getting local branches: %v\n", err)
		return
	}

	for _, branch := range locals {
		if branch == currentBranch {
			continue
		}
		tr, err := r.GetBranchTrackingRemote(branch)
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
		// checked out in another worktree -> show but mark unselectable;
		// `git checkout <branch>` would fail. dedupKey stays clean.
		if wt, ok := checkedOut[branch]; ok {
			entry.display += checkedOutSuffix(wt)
		}
		select {
		case queue <- entry:
		case <-ctx.Done():
			return
		}
	}
}

func streamRemoteBranches(ctx context.Context, r git.Repo, errOut io.Writer, queue chan<- branchEntry, remote, currentBranch, trackingRemote string, checkedOut map[string]string) {
	branches, err := r.GetRemoteBranches(remote)
	if err != nil {
		fmt.Fprintf(errOut, "error getting remote branches for '%s': %v\n", remote, err)
		return
	}

	for _, branch := range branches {
		if remote == trackingRemote && branch == currentBranch {
			continue
		}
		entry := branchEntry{
			display:  "remote: " + remote + "/" + branch,
			dedupKey: "remote:" + remote + "/" + branch,
		}
		// selecting a remote branch ends in `git checkout <branch>` on the
		// local landing name; if that's checked out elsewhere it'd fail, so
		// show but mark unselectable. skip the current branch (own ref).
		if wt, ok := checkedOut[branch]; ok && branch != currentBranch {
			entry.display += checkedOutSuffix(wt)
		}
		select {
		case queue <- entry:
		case <-ctx.Done():
			return
		}
	}
}
