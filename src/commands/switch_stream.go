package commands

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/lczyk/gitgum/internal/git"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

// Entries are paced into the picker so the list visibly fills rather than
// appearing at once -- cosmetic, and only worth anything for the first
// screenful. The cap keeps the cost from scaling with the repo: a couple of
// thousand remote branches would otherwise spend seconds mid-fill, matching
// anything typed against a list still filling up.
const (
	streamDelay        = 3 * time.Millisecond
	streamDelayEntries = 200
)

// checkedOutMarker tags a branch already checked out in some worktree. The
// switch flow always ends in `git checkout <name>` / reset, which git rejects
// for a branch checked out anywhere (including the current worktree -- you're
// already on it), so switch and delete show these entries but flag them
// unselectable. It's the shared prefix of both suffix variants ("in <wt>
// worktree)" and "here. HEAD)").
const checkedOutMarker = " (checked out "

// currentBranchMarker tags the branch checked out in the current worktree --
// the one you're on. The trailing "HEAD" is a search token: typing "HEAD" in
// the picker surfaces the current branch (a `branch` start point, the pull
// target in switch, and just visible in delete).
const currentBranchMarker = checkedOutMarker + "here. HEAD)"

// detachedMarker tags the entry standing in for a detached HEAD. It names
// where HEAD sits but is not a branch, so switch and delete can't act on it.
const detachedMarker = "HEAD (detached at "

// detachedEntry renders the picker row for a detached HEAD at short sha.
func detachedEntry(short string) string {
	return "local: " + detachedMarker + short + ")"
}

// checkedOutSuffix renders the display suffix for a branch checked out in
// another worktree (never the current one -- that branch gets
// currentBranchMarker instead, keyed on separately in streamLocalBranches).
func checkedOutSuffix(worktreePath string) string {
	return checkedOutMarker + "in " + filepath.Base(worktreePath) + " worktree)"
}

type branchKind int

const (
	localBranch   branchKind = iota // "local: <branch>", no upstream
	trackedBranch                   // "local/remote: <remote>/<branch>"
	remoteBranch                    // "remote: <remote>/<branch>", no local counterpart
	detachedHead                    // detachedEntry; branch holds the short sha
)

// branchPick is what a picker row stands for. The display text is for the
// user to read and search; the commands act on this.
type branchPick struct {
	kind   branchKind
	branch string // local branch name, remote branch name, or detached sha
	remote string // set for trackedBranch and remoteBranch
	// current marks the branch checked out in this worktree.
	current bool
}

// ref names the pick as git would: the remote-tracking ref for tracked and
// remote branches, the branch or commit otherwise.
func (p branchPick) ref() string {
	if p.remote != "" {
		return p.remote + "/" + p.branch
	}
	return p.branch
}

// branchFeed is the picker feed plus what each of its rows stands for.
type branchFeed struct {
	*ff.Feed
	mu    sync.Mutex
	picks map[ff.ID]branchPick
}

func newBranchFeed() *branchFeed {
	return &branchFeed{Feed: ff.NewFeed(), picks: map[ff.ID]branchPick{}}
}

// add holds mu across the Add, so a row can't be confirmed before its pick is
// recorded.
func (b *branchFeed) add(display string, p branchPick, flags ff.Flags) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.picks[b.AddWith(display, flags)] = p
}

func (b *branchFeed) pick(id ff.ID) (branchPick, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.picks[id]
	if !ok {
		return branchPick{}, fmt.Errorf("picked entry %d not in the branch list", id)
	}
	return p, nil
}

type branchEntry struct {
	display  string
	dedupKey string
	pick     branchPick
	flags    ff.Flags
}

// branchStreamOpts tunes streamBranches for its three consumers.
type branchStreamOpts struct {
	// includeCurrent emits the current branch. switch and delete keep it so it's
	// visible; branch keeps it because branching off HEAD is the common case.
	includeCurrent bool
	// markCheckedOut appends checkedOutSuffix to branches checked out in another
	// worktree and flags them, the current branch and a detached HEAD
	// unselectable. switch and delete want this -- git refuses both operations
	// on such a branch. branch does not: a branch checked out elsewhere is a
	// perfectly good start point, and so is a detached HEAD.
	markCheckedOut bool
	// pickCurrent keeps the current branch selectable under markCheckedOut:
	// in switch, picking it means "pull the branch you're on" rather than a
	// no-op re-checkout.
	pickCurrent bool
	// detachedAt is the short sha HEAD is detached at, "" when HEAD is on a
	// branch. Set, it prepends a row naming the commit, so the picker isn't
	// silent about where you actually are.
	detachedAt string
	// locals is the local-branch listing when the caller already has one --
	// delete reads it up front for its empty-repo guard. nil means read it
	// here, which is what a caller with no reason to look first does.
	locals []git.LocalBranch
}

// blocked returns the flags for a row that markCheckedOut would block.
func (o branchStreamOpts) blocked() ff.Flags {
	if o.markCheckedOut {
		return ff.Unselectable
	}
	return 0
}

// streamBranches collects local and remote branches concurrently, deduplicating
// and writing into a feed the picker consumes. The feed is closed once every
// producer has finished. Caller cancels ctx when the consumer (fuzzyfinder) is
// done; cancellation also stops producers. errOut receives non-fatal
// diagnostic messages from the producers.
func streamBranches(ctx context.Context, r git.Repo, errOut io.Writer, currentBranch, trackingRemote string, remotes []string, opts branchStreamOpts) *branchFeed {
	src := newBranchFeed()
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	queue := make(chan branchEntry, 1000)
	go func() {
		drained := 0
		for {
			select {
			case entry, ok := <-queue:
				if !ok {
					src.Close()
					return
				}
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
				src.add(entry.display, entry.pick, entry.flags)
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
		src.add(detachedEntry(opts.detachedAt), branchPick{kind: detachedHead, branch: opts.detachedAt}, opts.blocked())
	}

	checkedOut := streamLocalBranches(ctx, r, errOut, queue, currentBranch, opts)

	// One listing covers every remote: git lists them all whichever one you
	// ask about, so a producer per remote used to mean a full listing per
	// remote, each throwing away all but its own share.
	var byRemote map[string][]string
	if len(remotes) > 0 {
		var err error
		if byRemote, err = r.RemoteBranches(); err != nil {
			fmt.Fprintf(errOut, "error getting remote branches: %v\n", err)
			byRemote = nil
		}
	}
	var producers sync.WaitGroup
	for _, remote := range remotes {
		producers.Go(func() {
			streamRemoteBranches(ctx, queue, remote, byRemote[remote], currentBranch, trackingRemote, checkedOut)
		})
	}
	go func() {
		producers.Wait()
		close(queue)
	}()

	// Known limitation: branches deleted in another shell while the picker
	// is open stay in the list until the user closes and reopens it. The
	// feed supports Remove, but periodically polling git from inside the
	// picker isn't worth the complexity for a stale entry that fails loudly
	// on selection (`git checkout` errors). If this becomes a real
	// annoyance, hook a rescan goroutine here.
	return src
}

// streamLocalBranches returns the branch-to-worktree map its own listing
// already answers, so the remote producers can mark a branch checked out
// without a worktree listing of their own.
func streamLocalBranches(ctx context.Context, r git.Repo, errOut io.Writer, queue chan<- branchEntry, currentBranch string, opts branchStreamOpts) map[string]string {
	locals := opts.locals
	if locals == nil {
		var err error
		if locals, err = r.LocalBranches(); err != nil {
			fmt.Fprintf(errOut, "error getting local branches: %v\n", err)
			return nil
		}
	}
	checkedOut := make(map[string]string, len(locals))
	if opts.markCheckedOut {
		for _, local := range locals {
			if local.WorktreePath != "" {
				checkedOut[local.Name] = local.WorktreePath
			}
		}
	}

	for _, local := range locals {
		branch := local.Name
		if branch == currentBranch && !opts.includeCurrent {
			continue
		}
		tr := local.Remote()
		var entry branchEntry
		if tr != "" {
			// Include the tracking remote in the display so the row is
			// searchable by remote name (typing "origin" surfaces every branch
			// tracking origin), matching the "remote: <remote>/<branch>" rows.
			entry = branchEntry{
				display:  "local/remote: " + tr + "/" + branch,
				dedupKey: "remote:" + tr + "/" + branch,
				pick:     branchPick{kind: trackedBranch, remote: tr, branch: branch},
			}
		} else {
			entry = branchEntry{
				display:  "local: " + branch,
				dedupKey: "local:" + branch,
				pick:     branchPick{kind: localBranch, branch: branch},
			}
		}
		// The current branch always carries currentBranchMarker (the "HEAD"
		// search token) -- it's this worktree's checkout, so no other worktree
		// can also hold it. Any other branch checked out elsewhere gets the
		// worktree suffix and is blocked (switch/delete refuse it). dedupKey
		// stays clean either way.
		if branch == currentBranch {
			entry.display += currentBranchMarker
			entry.pick.current = true
			if !opts.pickCurrent {
				entry.flags = opts.blocked()
			}
		} else if wt, ok := checkedOut[branch]; ok {
			entry.display += checkedOutSuffix(wt)
			entry.flags = ff.Unselectable
		}
		select {
		case queue <- entry:
		case <-ctx.Done():
			return checkedOut
		}
	}
	return checkedOut
}

func streamRemoteBranches(ctx context.Context, queue chan<- branchEntry, remote string, branches []string, currentBranch, trackingRemote string, checkedOut map[string]string) {
	for _, branch := range branches {
		if remote == trackingRemote && branch == currentBranch {
			continue
		}
		entry := branchEntry{
			display:  "remote: " + remote + "/" + branch,
			dedupKey: "remote:" + remote + "/" + branch,
			pick:     branchPick{kind: remoteBranch, remote: remote, branch: branch},
		}
		// selecting a remote branch ends in `git checkout <branch>` on the
		// local landing name; if that's checked out elsewhere it'd fail, so
		// show but mark unselectable. skip the current branch (own ref).
		if wt, ok := checkedOut[branch]; ok && branch != currentBranch {
			entry.display += checkedOutSuffix(wt)
			entry.flags = ff.Unselectable
		}
		select {
		case queue <- entry:
		case <-ctx.Done():
			return
		}
	}
}
