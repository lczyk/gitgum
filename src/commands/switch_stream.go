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

// streamDelay paces entries into the picker so the list visibly fills rather
// than appearing all at once. Purely cosmetic.
const streamDelay = 3 * time.Millisecond

// streamDelayEntries bounds how many entries pay streamDelay. The effect is
// only worth anything for the first screenful or so, but the cost is linear:
// a repo with a couple of thousand remote branches would otherwise take
// len(branches) * streamDelay -- seconds -- before the list was complete, and
// a query typed in the meantime would match against a list still filling up.
// Past this many entries the remainder is added as fast as it arrives.
const streamDelayEntries = 200

// checkedOutMarker tags a branch already checked out in some worktree. The
// switch flow always ends in `git checkout <name>` / reset, which git rejects
// for a branch checked out anywhere (including the current worktree -- you're
// already on it), so these entries are shown but made unselectable (see
// switch.go). It's the shared prefix of both suffix variants ("in <wt>
// worktree)" and "here. HEAD)"), so the emitter and the unselectable predicate
// stay in sync via this const.
const checkedOutMarker = " (checked out "

// currentBranchMarker tags the branch checked out in the current worktree --
// the one you're on. The trailing "HEAD" is a search token: typing "HEAD" in
// the picker surfaces the current branch (a `branch` start point, and just
// visible in switch/delete). It shares checkedOutMarker's prefix, so switch and
// delete still treat it as unselectable; `branch` leaves it selectable and
// strips the marker back off in parseBranchEntry.
const currentBranchMarker = checkedOutMarker + "here. HEAD)"

// detachedMarker tags the entry standing in for a detached HEAD. It names
// where HEAD sits but is not a branch, so it can't be switched to; like
// checkedOutMarker, the emitter and the unselectable predicate both key on it.
const detachedMarker = "HEAD (detached at "

// detachedEntry renders the picker row for a detached HEAD at short sha.
func detachedEntry(short string) string {
	return "local: " + detachedMarker + short + ")"
}

// detachedEntrySHA recovers the short sha from a detached-HEAD picker payload
// ("HEAD (detached at cf10b5f)"), the name half of a detachedEntry row.
func detachedEntrySHA(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, detachedMarker)
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(rest, ")"), true
}

// checkedOutSuffix renders the display suffix for a branch checked out in
// another worktree (never the current one -- that branch gets
// currentBranchMarker instead, keyed on separately in streamLocalBranches).
func checkedOutSuffix(worktreePath string) string {
	return checkedOutMarker + "in " + filepath.Base(worktreePath) + " worktree)"
}

// isUnselectable is the picker's Unselectable predicate: an entry carrying
// either marker names something `git checkout` would refuse -- a branch checked
// out in another worktree, or a detached HEAD, which is not a branch at all.
func isUnselectable(item string) bool {
	return strings.Contains(item, checkedOutMarker) || strings.Contains(item, detachedMarker)
}

// switchUnselectable is switch's predicate. It's isUnselectable with one
// exception: the current branch (currentBranchMarker) is selectable -- picking
// it means "pull the branch you're on" rather than a no-op re-checkout. Other
// worktrees' checkouts and a detached HEAD stay blocked.
func switchUnselectable(item string) bool {
	if strings.Contains(item, currentBranchMarker) {
		return false
	}
	return isUnselectable(item)
}

// parseBranchEntry splits a picker payload ("local: foo", "remote: origin/foo",
// "local/remote: origin/foo") into its type tag and name. The name is whatever
// followed the tag, suffixes included -- callers that enabled markCheckedOut
// only ever see entries they made selectable, so no checked-out suffix reaches
// here. A detachedEntry does: `branch` lets you cut a branch off HEAD, and
// startPointFor unwraps the name via detachedEntrySHA.
func parseBranchEntry(selected string) (typ, name string, err error) {
	typ, name, ok := strings.Cut(selected, ": ")
	if !ok {
		return "", "", fmt.Errorf("invalid selection: %s", selected)
	}
	// The current branch is selectable in `branch` (branch off HEAD) and carries
	// currentBranchMarker for search; strip it so the name is the bare ref. Git
	// branch names can't contain spaces, so the marker can't be part of one.
	name = strings.TrimSuffix(name, currentBranchMarker)
	return typ, name, nil
}

// localRemoteBranch recovers the local branch name from a "local/remote"
// picker payload, which is "<remote>/<branch>" (see streamLocalBranches). A
// git remote name never contains '/', so a single cut on the first '/' yields
// the branch -- which may itself contain '/' (e.g. "feat/foo"). Returns false
// if the payload has no '/'.
func localRemoteBranch(name string) (string, bool) {
	_, branch, ok := strings.Cut(name, "/")
	return branch, ok
}

type branchEntry struct {
	display  string
	dedupKey string
}

// branchStreamOpts tunes streamBranches for its three consumers.
type branchStreamOpts struct {
	// includeCurrent emits the current branch. switch and delete keep it so it's
	// visible but unselectable (the checked-out marker for this worktree blocks
	// it); branch keeps it because branching off HEAD is the common case.
	includeCurrent bool
	// markCheckedOut appends checkedOutSuffix to branches checked out in another
	// worktree, which isUnselectable then blocks. switch and delete want
	// this -- git refuses both operations on such a branch. branch does not: a
	// branch checked out elsewhere is a perfectly good start point, and the
	// suffix would otherwise have to be stripped back off the picker payload.
	markCheckedOut bool
	// detachedAt is the short sha HEAD is detached at, "" when HEAD is on a
	// branch. Set, it prepends an unselectable row naming the commit, so the
	// picker isn't silent about where you actually are.
	detachedAt string
}

// streamBranches collects local and remote branches concurrently, deduplicating
// and writing into a SliceSource the picker consumes. Caller cancels ctx
// when the consumer (fuzzyfinder) is done; cancellation also stops producers.
// errOut receives non-fatal diagnostic messages from the producers.
//
// The returned SliceSource supports both Add (used by the producers below)
// and RemoveFunc (left available for future hooks that drop branches as the
// user deletes them).
func streamBranches(ctx context.Context, r git.Repo, errOut io.Writer, currentBranch, trackingRemote string, remotes []string, opts branchStreamOpts) *ff.SliceSource {
	src := ff.NewSliceSource()
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	// Fetch checkout state once so producers can do map lookups instead of
	// N subprocess calls. Skipped entirely when nobody's going to mark them.
	checkedOut := map[string]string{}
	if opts.markCheckedOut {
		var err error
		if checkedOut, err = r.CheckedOutBranches(); err != nil {
			fmt.Fprintf(errOut, "error getting worktrees: %v\n", err)
			checkedOut = map[string]string{}
		}
	}

	queue := make(chan branchEntry, 1000)
	go func() {
		drained := 0
		for {
			select {
			case entry := <-queue:
				if drained < streamDelayEntries {
					time.Sleep(streamDelay)
					drained++
				}
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

	// Local branches are scanned synchronously (no network involved) before the
	// remote goroutines start. This guarantees local entries queue -- and thus
	// win the dedup race in the consumer above -- ahead of any remote entry
	// sharing the same dedupKey. Without this ordering, a branch that's both
	// local and remote-tracked could race-display as "remote: ..." instead of
	// "local/remote: ...", sending switch down the fetch/tracking-branch path
	// for a branch that already exists locally.
	if opts.detachedAt != "" {
		src.Add(detachedEntry(opts.detachedAt))
	}

	streamLocalBranches(ctx, r, errOut, queue, currentBranch, checkedOut, opts.includeCurrent)
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

func streamLocalBranches(ctx context.Context, r git.Repo, errOut io.Writer, queue chan<- branchEntry, currentBranch string, checkedOut map[string]string, includeCurrent bool) {
	locals, err := r.GetLocalBranches()
	if err != nil {
		fmt.Fprintf(errOut, "error getting local branches: %v\n", err)
		return
	}

	for _, branch := range locals {
		if branch == currentBranch && !includeCurrent {
			continue
		}
		tr, err := r.GetBranchTrackingRemote(branch)
		if err != nil {
			tr = ""
		}
		var entry branchEntry
		if tr != "" {
			// Include the tracking remote in the display so the row is
			// searchable by remote name (typing "origin" surfaces every branch
			// tracking origin), matching the "remote: <remote>/<branch>" rows.
			// applySelection/applyDeletion strip the remote back off via
			// localRemoteBranch to recover the local branch to check out/delete.
			entry = branchEntry{
				display:  "local/remote: " + tr + "/" + branch,
				dedupKey: "remote:" + tr + "/" + branch,
			}
		} else {
			entry = branchEntry{
				display:  "local: " + branch,
				dedupKey: "local:" + branch,
			}
		}
		// The current branch always carries currentBranchMarker (the "HEAD"
		// search token) -- it's this worktree's checkout, so no other worktree
		// can also hold it. Any other branch checked out elsewhere gets the
		// worktree suffix, which isUnselectable then blocks (switch/delete refuse
		// it). dedupKey stays clean either way.
		if branch == currentBranch {
			entry.display += currentBranchMarker
		} else if wt, ok := checkedOut[branch]; ok {
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
