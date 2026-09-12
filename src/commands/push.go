package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"

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
		// Refresh the remote-tracking ref before comparing. It's a local cache;
		// pushing against a stale one is exactly what produces the surprise
		// non-fast-forward rejection this flow exists to catch ahead of time.
		if remote, _, ok := strings.Cut(remoteBranch, "/"); ok {
			if err := p.repo().Fetch(remote, ""); err != nil {
				return err
			}
		}

		localCommit, err := p.repo().GetCommitHash(currentBranch)
		if err != nil {
			return fmt.Errorf("getting local commit: %w", err)
		}
		remoteCommit, err := p.repo().GetCommitHash(remoteBranch)
		if err != nil {
			// The fetch above prunes a remote-tracking ref whose upstream was
			// deleted, so it may no longer resolve. That's the deleted-upstream
			// case -- offer to recreate rather than failing on a missing ref.
			if handled, herr := p.handleStaleUpstream(currentBranch, remoteBranch); herr != nil {
				return herr
			} else if handled {
				return nil
			}
			return fmt.Errorf("getting remote commit: %w", err)
		}
		if localCommit == remoteCommit {
			// remoteBranch is the local remote-tracking ref (refs/remotes/...),
			// which can be stale when the fetch above was skipped (no remote in
			// the ref name). Verify it still exists before declaring "up to date".
			if handled, err := p.handleStaleUpstream(currentBranch, remoteBranch); err != nil {
				return err
			} else if handled {
				return nil
			}
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
			return p.reconcileDiverged(currentBranch, remoteBranch)
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

	if !p.repo().RemoteBranchExists(selectedRemote, currentBranch) {
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

// handleStaleUpstream is called when the local branch matches its
// remote-tracking ref, which would normally mean "nothing to push". Because
// that ref is a local cache, it can lie when the upstream branch was deleted
// remotely without a prune. handleStaleUpstream does a live check: if the
// upstream is gone (remote reachable, ref missing) it offers to recreate it;
// if the remote is unreachable it warns and lets the caller fall through to
// the (best-effort) up-to-date message. Returns handled=true when it has
// fully dealt with the push (recreated it, or offered the other remotes on a
// decline).
func (p *PushCommand) handleStaleUpstream(currentBranch, upstream string) (handled bool, err error) {
	remote, branch, ok := strings.Cut(upstream, "/")
	if !ok {
		return false, nil
	}
	exists, reachable := p.repo().RemoteBranchReachability(remote, branch)
	if exists {
		return false, nil
	}
	if !reachable {
		fmt.Fprintf(p.err(), "Warning: could not reach remote '%s' to verify branch '%s' (network/auth issue?).\n", remote, branch)
		return false, nil
	}

	fmt.Fprintf(p.out(), "Branch '%s' no longer exists on remote '%s' (deleted upstream). Local tracking info is stale.\n", branch, upstream)
	confirmed, err := p.sel().Confirm(fmt.Sprintf("Recreate remote branch '%s'?", upstream), true)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return false, err
		}
		return false, fmt.Errorf("confirming recreate upstream: %w", err)
	}
	if !confirmed {
		return true, p.offerOtherRemotes(currentBranch, []string{remote}, nil)
	}
	if err := p.repo().RunWriteStream("push", "-u", remote, currentBranch); err != nil {
		return false, fmt.Errorf("failed to push: %w", err)
	}
	fmt.Fprintf(p.out(), "Recreated remote branch '%s'.\n", upstream)
	return true, nil
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
//     rebases and pushes the result.
//
// Declining, or a rebase that won't apply, offers the other remotes instead;
// with none left, the latter is an error. It owns the whole remote-ahead case:
// the caller returns whatever this returns.
func (p *PushCommand) reconcileDiverged(currentBranch, upstream string) error {
	upstreamRemote, _, _ := strings.Cut(upstream, "/")
	localAhead, err := p.repo().IsBranchAheadOfRemote(currentBranch, upstream)
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
		cleanup, err := handleDirtyTree(&p.cmdIO, "push")
		if err != nil {
			return err
		}
		defer cleanup()
		if err := p.repo().Integrate(git.PullFFOnly, upstream); err != nil {
			return err
		}
		fmt.Fprintf(p.out(), "Fast-forwarded '%s' to '%s'. Nothing to push.\n", currentBranch, upstream)
		return nil
	}

	// Genuine divergence. Only offer the rebase if it would apply cleanly --
	// checked in memory (no working-tree / index / HEAD changes) so a would-be
	// conflict never leaves the repo mid-rebase.
	conflict, err := p.repo().WouldRebaseConflict(upstream, currentBranch)
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

	cleanup, err := handleDirtyTree(&p.cmdIO, "push")
	if err != nil {
		return err
	}
	defer cleanup()

	if err := p.repo().Integrate(git.PullRebase, upstream); err != nil {
		return err
	}
	if err := p.repo().Push(); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}
	fmt.Fprintf(p.out(), "Rebased '%s' onto '%s' and pushed.\n", currentBranch, upstream)
	return nil
}
