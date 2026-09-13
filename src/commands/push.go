package commands

import (
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/lczyk/gitgum/internal/git"
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
	currentBranch, err := p.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if currentBranch == "HEAD" {
		return fmt.Errorf("HEAD is detached: check out a branch to push")
	}

	// A named remote overrides the upstream -- unless it names the upstream
	// itself, which is just a plain `gg push`.
	if len(args) > 0 {
		remote, err := p.selectRemote(currentBranch, args[0])
		if err != nil {
			return err
		}
		if remote+"/"+currentBranch != remoteBranch {
			return p.pushToRemote(remote, currentBranch, nil)
		}
	}

	// When git names no destination for a plain push (an upstream of another
	// name under push.default=simple -- usually a PR base -- push.default=nothing,
	// a local upstream), ask where to push as if there were no upstream.
	if remoteBranch != "" {
		target, err := p.repo().GetBranchPushTarget(currentBranch)
		if err != nil {
			return fmt.Errorf("getting push target: %w", err)
		}
		if target == "" {
			fmt.Fprintf(p.out(), "Branch '%s' tracks '%s', but git has no push destination for it.\n",
				currentBranch, remoteBranch)
			remoteBranch = ""
		}
	}

	if remoteBranch != "" {
		// Ask the remote where the upstream is now: the remote-tracking ref is a
		// local cache, and pushing against a stale one is exactly what produces
		// the surprise non-fast-forward rejection this flow exists to catch. A
		// local upstream (no remote in the name) has no one to ask.
		if remote, branch, ok := strings.Cut(remoteBranch, "/"); ok {
			_, exists, err := p.probeBranch(remote, branch)
			if err != nil {
				return p.offerOtherRemotes(currentBranch, []string{remote}, err)
			}
			if !exists {
				return p.offerRecreate(currentBranch, remote, branch)
			}
		}

		localCommit, err := p.repo().GetCommitHash(currentBranch)
		if err != nil {
			return fmt.Errorf("getting local commit: %w", err)
		}
		remoteCommit, err := p.repo().GetCommitHash(remoteBranch)
		if err != nil {
			return fmt.Errorf("getting remote commit: %w", err)
		}
		if localCommit == remoteCommit {
			fmt.Fprintf(p.out(), "No changes to push. Local branch '%s' is up to date with '%s'.\n",
				currentBranch, remoteBranch)
			return nil
		}

		// Does the remote hold commits we lack? If so a plain push is rejected
		// as non-fast-forward; reconcileDiverged owns that case (offer a rebase
		// when it's clean, error when it isn't).
		remoteAhead, err := p.repo().IsBranchAheadOfRemote(remoteBranch, currentBranch)
		if err != nil {
			return fmt.Errorf("checking divergence: %w", err)
		}
		if remoteAhead {
			return p.reconcileDiverged(currentBranch, remoteBranch, remoteCommit)
		}

		// Local is strictly ahead: a plain push fast-forwards the remote.
		fmt.Fprintf(p.out(), "Current branch already has a remote tracking branch: %s\n", remoteBranch)
		p.showPushDelta(remoteCommit, localCommit)
		confirmed, err := p.sel().Confirm("Do you want to push to the remote tracking branch?", true)
		if err != nil {
			return fmt.Errorf("confirming push to upstream: %w", err)
		}
		if !confirmed {
			upstreamRemote, _, _ := strings.Cut(remoteBranch, "/")
			return p.offerOtherRemotes(currentBranch, []string{upstreamRemote}, nil)
		}
		if err := p.repo().Push(); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Fprintf(p.out(), "Pushed to remote tracking branch '%s'.\n", remoteBranch)
		return nil
	}

	remote, err := p.selectRemote(currentBranch, "")
	if err != nil {
		return err
	}
	return p.pushToRemote(remote, currentBranch, nil)
}

// selectRemote picks the remote to push currentBranch to: name, when it is
// one, otherwise the picker with name as its query.
func (p *PushCommand) selectRemote(currentBranch, name string) (string, error) {
	remotes, err := p.repo().GetRemotes()
	if err != nil {
		return "", fmt.Errorf("getting remotes: %w", err)
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("no remotes")
	}
	if slices.Contains(remotes, name) {
		return name, nil
	}

	var query []string
	if name != "" {
		query = []string{name}
	}
	remote, err := p.sel().Select(fmt.Sprintf("Push '%s' to", currentBranch), p.rankRemotes(remotes), query...)
	if err != nil {
		return "", fmt.Errorf("selecting remote: %w", err)
	}
	return remote, nil
}

// rankRemotes puts the remotes most local branches track first -- where a user
// usually pushes -- keeping git's order among equals. A failed count leaves
// the order as it was.
func (p *PushCommand) rankRemotes(remotes []string) []string {
	counts, err := p.repo().TrackedRemoteCounts()
	if err != nil {
		return remotes
	}
	ranked := slices.Clone(remotes)
	slices.SortStableFunc(ranked, func(a, b string) int { return counts[b] - counts[a] })
	return ranked
}

// pushToRemote pushes currentBranch to selectedRemote, prompting before
// creating a missing remote branch or pushing to an existing one. Whichever
// path runs, the branch ends up tracking the remote it was pushed to.
// Declining offers the remotes not already in ruledOut.
func (p *PushCommand) pushToRemote(selectedRemote, currentBranch string, ruledOut []string) error {
	expectedRemoteBranchName := selectedRemote + "/" + currentBranch

	remoteCommit, exists, err := p.probeBranch(selectedRemote, currentBranch)
	if err != nil {
		return p.offerOtherRemotes(currentBranch, slices.Concat(ruledOut, []string{selectedRemote}), err)
	}
	if !exists {
		confirmed, err := p.sel().Confirm(fmt.Sprintf("No remote branch '%s' found. Do you want to create it?",
			expectedRemoteBranchName), false)
		if err != nil {
			return fmt.Errorf("confirming create remote branch: %w", err)
		}
		if !confirmed {
			return p.offerOtherRemotes(currentBranch, slices.Concat(ruledOut, []string{selectedRemote}), nil)
		}

		if err := p.repo().RunWriteStream("push", "-u", selectedRemote, currentBranch); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		fmt.Fprintf(p.out(), "Created and set tracking reference for '%s' to '%s'.\n",
			currentBranch, expectedRemoteBranchName)
		return nil
	}

	localCommit, err := p.repo().GetCommitHash(currentBranch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
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

	p.showPushDelta(remoteCommit, localCommit)
	confirmed, err := p.sel().Confirm(fmt.Sprintf("Remote branch '%s' already exists. Do you want to push to it?",
		expectedRemoteBranchName), true)
	if err != nil {
		return fmt.Errorf("confirming push to remote: %w", err)
	}
	if !confirmed {
		return p.offerOtherRemotes(currentBranch, slices.Concat(ruledOut, []string{selectedRemote}), nil)
	}

	if err := p.repo().RunWriteStream("push", "-u", selectedRemote, currentBranch); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	fmt.Fprintf(p.out(), "Pushed to remote branch '%s' and updated upstream to it.\n", expectedRemoteBranchName)
	return nil
}

// offerOtherRemotes runs once the push can't go where it was headed -- the
// user declined that destination, or push can't reconcile with it -- and
// offers every remote not in ruledOut, pushing to the one picked and moving
// the upstream there. With none left, stuck is the outcome; a plain decline
// passes nil and just says nothing was pushed.
func (p *PushCommand) offerOtherRemotes(currentBranch string, ruledOut []string, stuck error) error {
	remotes, err := p.repo().GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}
	others := make([]string, 0, len(remotes))
	for _, r := range remotes {
		if !slices.Contains(ruledOut, r) {
			others = append(others, r)
		}
	}
	if len(others) == 0 {
		if stuck != nil {
			return stuck
		}
		fmt.Fprintln(p.out(), "Nothing pushed.")
		return nil
	}
	if stuck != nil {
		fmt.Fprintf(p.err(), "%s %v.\n", paint(ansiBoldYellow, "note:"), stuck)
	}

	remote, err := p.sel().Select(fmt.Sprintf("Push '%s' to", currentBranch), p.rankRemotes(others))
	if err != nil {
		return fmt.Errorf("selecting remote: %w", err)
	}
	return p.pushToRemote(remote, currentBranch, ruledOut)
}

// showPushDelta prints a compact-summary of the commits about to be pushed
// (remote tip -> local tip) in the same style as `gg diff`, so the user sees
// what they're sending before confirming. Best-effort: a render hiccup never
// blocks the push.
func (p *PushCommand) showPushDelta(remoteCommit, localCommit string) {
	summary, err := diffSummary(p.repo(), 0, remoteCommit+".."+localCommit)
	if err == nil && summary != "" {
		fmt.Fprintln(p.out(), summary)
	}
}

// probeBranch asks remote for branch's tip and brings the remote-tracking ref
// in line with it, so what follows compares against the remote as it is now:
// one round trip, plus a fetch of that branch alone when its tip is new here.
func (p *PushCommand) probeBranch(remote, branch string) (tip string, exists bool, err error) {
	tip, exists, err = p.repo().RemoteBranchTip(remote, branch)
	if err != nil {
		return "", false, fmt.Errorf("could not reach remote '%s': %w", remote, err)
	}
	if !exists {
		return "", false, nil
	}
	if err := p.repo().SyncTrackingRef(remote, branch, tip); err != nil {
		return "", false, err
	}
	return tip, true, nil
}

// offerRecreate runs when the upstream branch is gone from its remote: it
// offers to push the branch back there or, declined, the other remotes.
func (p *PushCommand) offerRecreate(currentBranch, remote, branch string) error {
	upstream := remote + "/" + branch
	fmt.Fprintf(p.out(), "Branch '%s' no longer exists on remote '%s' (deleted upstream). Local tracking info is stale.\n", branch, remote)
	confirmed, err := p.sel().Confirm(fmt.Sprintf("Recreate remote branch '%s'?", upstream), true)
	if err != nil {
		return fmt.Errorf("confirming recreate upstream: %w", err)
	}
	if !confirmed {
		return p.offerOtherRemotes(currentBranch, []string{remote}, nil)
	}
	if err := p.repo().RunWriteStream("push", "-u", remote, currentBranch); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	fmt.Fprintf(p.out(), "Recreated remote branch '%s'.\n", upstream)
	return nil
}

// reconcileDiverged handles a push where the remote-tracking ref holds commits
// the local branch lacks, so a plain push would be rejected as
// non-fast-forward. Rather than let the user run into that rejection, it works
// out whether a `pull --rebase` would resolve it cleanly:
//   - local strictly behind (no local commits the upstream lacks): the rebase
//     is a pure fast-forward -- always clean, but it leaves nothing to push.
//     Offer to catch up.
//   - genuinely diverged (each side has unique commits): offer the rebase only
//     if it would apply without conflicts (checked in memory, no side effects),
//     rather than drop the user into a half-finished rebase. On confirmation it
//     rebases onto the tip it checked and pushes the result; a rebase that
//     stops part-way is aborted, and a push that fails undoes the rebase
//     unless the remote turns out to have it. Ctrl-C part-way through is a
//     cancel, undone the same way.
//
// Declining, or a rebase that won't apply, offers the other remotes instead;
// with none left, the latter is an error. It owns the whole remote-ahead case:
// the caller returns whatever this returns.
func (p *PushCommand) reconcileDiverged(currentBranch, upstream, remoteTip string) error {
	upstreamRemote, _, _ := strings.Cut(upstream, "/")
	localAhead, err := p.repo().IsBranchAheadOfRemote(currentBranch, remoteTip)
	if err != nil {
		return fmt.Errorf("checking divergence: %w", err)
	}

	if !localAhead {
		fmt.Fprintf(p.out(), "Local branch '%s' is behind '%s'; nothing to push until you catch up.\n",
			currentBranch, upstream)
		confirmed, err := p.sel().Confirm(
			fmt.Sprintf("Pull --rebase to fast-forward '%s' onto '%s'?", currentBranch, upstream), true)
		if err != nil {
			return fmt.Errorf("confirming rebase: %w", err)
		}
		if !confirmed {
			return p.offerOtherRemotes(currentBranch, []string{upstreamRemote}, nil)
		}
		apply, err := decideDirtyTree(&p.cmdIO, "push")
		if err != nil {
			return err
		}
		intr := catchInterrupts()
		defer intr.stop()
		cleanup, err := apply()
		if err != nil {
			return err
		}
		defer cleanup()
		before, err := p.repo().GetCommitHash(currentBranch)
		if err != nil {
			return fmt.Errorf("getting local commit: %w", err)
		}
		if err := p.repo().Integrate(git.PullFFOnly, remoteTip); err != nil {
			return intr.or(err)
		}
		if intr.seen() {
			return p.undoInterrupted(currentBranch, before, "fast-forward")
		}
		fmt.Fprintf(p.out(), "Fast-forwarded '%s' to '%s'. Nothing to push.\n", currentBranch, upstream)
		return nil
	}

	// Genuine divergence. Only offer the rebase if it would apply cleanly --
	// checked in memory (no working-tree / index / HEAD changes) so a would-be
	// conflict never leaves the repo mid-rebase.
	conflict, err := p.repo().WouldRebaseConflict(remoteTip, currentBranch)
	if err != nil {
		return p.offerOtherRemotes(currentBranch, []string{upstreamRemote}, fmt.Errorf(
			"remote '%s' has diverged from '%s'; could not check whether a rebase applies cleanly (%w). "+
				"Run `gg pull` to integrate manually", upstream, currentBranch, err))
	}
	if conflict {
		return p.offerOtherRemotes(currentBranch, []string{upstreamRemote}, fmt.Errorf(
			"remote '%s' has diverged from '%s' and a rebase would conflict. "+
				"Run `gg pull` to integrate manually", upstream, currentBranch))
	}

	confirmed, err := p.sel().Confirm(
		fmt.Sprintf("Remote '%s' has diverged from local '%s'. Pull --rebase before pushing?", upstream, currentBranch), true)
	if err != nil {
		return fmt.Errorf("confirming rebase: %w", err)
	}
	if !confirmed {
		return p.offerOtherRemotes(currentBranch, []string{upstreamRemote}, nil)
	}

	apply, err := decideDirtyTree(&p.cmdIO, "push")
	if err != nil {
		return err
	}
	intr := catchInterrupts()
	defer intr.stop()
	cleanup, err := apply()
	if err != nil {
		return err
	}
	// Deferred, so the stash comes back after any undo below has put the
	// branch back where it was.
	defer cleanup()

	before, err := p.repo().GetCommitHash(currentBranch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}
	if err := p.repo().Integrate(git.PullRebase, remoteTip); err != nil {
		return intr.or(p.abortRebase(currentBranch, err))
	}
	if intr.seen() {
		return p.undoInterrupted(currentBranch, before, "rebase")
	}
	if err := p.repo().Push(); err != nil {
		return intr.or(p.undoRebase(currentBranch, upstream, before, remoteTip, err))
	}
	fmt.Fprintf(p.out(), "Rebased '%s' onto '%s' and pushed.\n", currentBranch, upstream)
	return nil
}

// abortRebase backs out of a rebase that stopped part-way -- a conflict the
// in-memory check missed, or a hook refusing a commit -- so the branch is left
// as it was rather than mid-rebase.
func (p *PushCommand) abortRebase(currentBranch string, rebaseErr error) error {
	if op, yes := p.repo().InProgress(); !yes || !strings.Contains(op, "rebase") {
		return rebaseErr
	}
	if err := p.repo().RebaseAbort(); err != nil {
		return fmt.Errorf("%w; aborting the rebase failed too (%v): finish or abort it by hand", rebaseErr, err)
	}
	fmt.Fprintf(p.err(), "The rebase stopped part-way; aborted it, so '%s' is as it was.\n", currentBranch)
	return rebaseErr
}

// undoRebase runs when the push after a rebase failed. The push may have
// landed anyway -- a connection can drop after the remote took it -- so the
// remote is asked first: undoing a rebase it already has would leave the
// branch diverged from it all over again.
func (p *PushCommand) undoRebase(currentBranch, upstream, before, remoteTip string, pushErr error) error {
	pushErr = fmt.Errorf("failed to push: %w", pushErr)
	pushed, err := p.repo().GetCommitHash(currentBranch)
	if err != nil {
		return pushErr
	}
	var tip string
	if remote, branch, ok := strings.Cut(upstream, "/"); ok {
		tip, _, _ = p.repo().RemoteBranchTip(remote, branch)
	}
	switch tip {
	case pushed:
		fmt.Fprintf(p.out(), "Rebased '%s' onto '%s' and pushed; git reported an error, but the remote has it.\n",
			currentBranch, upstream)
		return nil
	case remoteTip:
		if err := p.repo().ResetKeep(before); err != nil {
			return fmt.Errorf("%w; undoing the rebase failed too (%v): undo it by hand with git reset --keep %s",
				pushErr, err, before)
		}
		fmt.Fprintf(p.err(), "Undid the rebase: '%s' is back where it was.\n", currentBranch)
		return pushErr
	default:
		fmt.Fprintf(p.err(), "Couldn't tell whether the push reached '%s', so the rebase stays; to undo it: git reset --keep %s\n",
			upstream, before)
		return pushErr
	}
}

// undoInterrupted takes back a step that finished before a Ctrl-C was noticed,
// so the cancel leaves the branch where it started.
func (p *PushCommand) undoInterrupted(currentBranch, before, step string) error {
	if err := p.repo().ResetKeep(before); err != nil {
		return fmt.Errorf("interrupted, and undoing the %s failed too (%v): undo it by hand with git reset --keep %s",
			step, err, before)
	}
	fmt.Fprintf(p.err(), "Interrupted: undid the %s, so '%s' is back where it was.\n", step, currentBranch)
	return ui.ErrCancelled
}

// interrupts catches Ctrl-C while push is changing things, so gg lives to undo
// what it has done instead of dying part-way; git, sent the same signal, still
// stops.
type interrupts struct {
	ch  chan os.Signal
	hit bool
}

func catchInterrupts() *interrupts {
	i := &interrupts{ch: make(chan os.Signal, 1)}
	signal.Notify(i.ch, os.Interrupt)
	return i
}

func (i *interrupts) stop() { signal.Stop(i.ch) }

// seen reports whether a Ctrl-C has arrived since catching began.
func (i *interrupts) seen() bool {
	if !i.hit {
		select {
		case <-i.ch:
			i.hit = true
		default:
		}
	}
	return i.hit
}

// or turns a failure into a cancel when a Ctrl-C caused it. The signal can
// trail the failure it caused by a moment, so it is given one.
func (i *interrupts) or(err error) error {
	if err == nil {
		return nil
	}
	if !i.hit {
		select {
		case <-i.ch:
			i.hit = true
		case <-time.After(100 * time.Millisecond):
		}
	}
	if i.hit {
		return ui.ErrCancelled
	}
	return err
}
