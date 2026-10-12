package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/pr"
)

type PullCommand struct {
	cmdIO
}

// pullModes lists the integration strategies the picker offers once local and
// upstream have diverged, in display order. A fast-forward is not among them:
// it cannot succeed on a diverged branch, and that is the only case the picker
// is shown for. Rebase leads as the one that keeps history linear without
// discarding anything.
var pullModes = []git.PullMode{git.PullRebase, git.PullMerge, git.PullReset}

// pullLeaveLabel is the picker's way out that is not a cancel: the branch is
// left as it is and the command ends having done nothing, exit 0.
const pullLeaveLabel = "leave as is"

// errNoUpstream is the sentinel pullBranch returns when a non-PR branch has no
// upstream. The `gg pull` command turns it into a user-facing error; `gg switch`
// (which pulls after landing on a branch) treats it as benign -- a local-only
// branch simply has nothing to pull.
var errNoUpstream = errors.New("no upstream configured")

func (p *PullCommand) Execute(args []string) error {
	if err := p.repo().CheckInRepo(); err != nil {
		return err
	}

	currentBranch, err := p.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	err = p.pullBranch(currentBranch)
	if errors.Is(err, errNoUpstream) {
		return fmt.Errorf("no upstream configured for '%s'", localRef(currentBranch))
	}
	return err
}

// pullBranch fetches and integrates the currently checked-out branch. It is the
// reusable core behind both `gg pull` and `gg switch`'s post-checkout update.
// The branch must already be checked out (git ops act on the working tree).
// Returns errNoUpstream for a non-PR branch with no upstream, leaving the
// caller to decide whether that's an error.
func (p *PullCommand) pullBranch(currentBranch string) error {
	// A checkout-pr branch has no normal upstream -- it mirrors a PR ref that
	// git can't track. Re-fetch that ref instead of erroring on "no upstream".
	if meta, ok, err := pr.ReadMeta(p.repo(), currentBranch); err != nil {
		return err
	} else if ok {
		return p.pullPR(currentBranch, meta)
	}

	upstream, err := p.repo().GetCurrentBranchUpstream()
	if err != nil {
		return fmt.Errorf("getting upstream: %w", err)
	}
	if upstream == "" {
		return errNoUpstream
	}
	remote, remoteBranch, ok := strings.Cut(upstream, "/")
	if !ok {
		return fmt.Errorf("unexpected upstream format: %s", upstream)
	}

	// Fetch first (empty refspec -> configured refspecs), so the divergence
	// check below reads the freshly-updated remote-tracking ref rather than a
	// stale cache. On a shallow clone this grabs new tip commits only.
	if err := p.repo().Fetch(remote, ""); err != nil {
		return err
	}
	if err := p.ensureUpstreamRef(remote, remoteBranch, upstream); err != nil {
		return err
	}

	localCommit, err := p.repo().GetCommitHash(currentBranch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}
	remoteCommit, err := p.repo().GetCommitHash(upstream)
	if err != nil {
		return fmt.Errorf("getting upstream commit: %w", err)
	}
	if localCommit == remoteCommit {
		fmt.Fprintf(p.out(), "Already up to date. '%s' matches '%s'.\n", localRef(currentBranch), upstream)
		return nil
	}

	// Only a genuine divergence justifies the strategy picker. Work out where
	// local sits relative to upstream:
	//   - strictly behind: ff-only / rebase / merge all collapse to the same
	//     fast-forward, so the picker would offer three labels for one outcome.
	//     Skip it and fast-forward directly.
	//   - strictly ahead: nothing upstream to integrate at all.
	//   - diverged (each side has unique commits): the mode changes the result,
	//     so ask.
	ahead, behind, err := p.repo().AheadBehind(currentBranch, upstream)
	if err != nil {
		return fmt.Errorf("checking divergence: %w", err)
	}
	localAhead, upstreamAhead := ahead > 0, behind > 0
	if localAhead && !upstreamAhead {
		fmt.Fprintf(p.out(), "Nothing to pull. '%s' is ahead of '%s'.\n", localRef(currentBranch), upstream)
		return nil
	}

	mode := git.PullFFOnly // strictly behind: every mode fast-forwards.
	if localAhead {        // diverged: let the user pick how to reconcile.
		mode, err = p.selectMode(currentBranch, upstream, ahead, behind)
		if err != nil {
			return err
		}
		if mode == pullLeave {
			fmt.Fprintf(p.out(), "Left '%s' as it is: ahead %d, behind %d of '%s'.\n",
				localRef(currentBranch), ahead, behind, upstream)
			return nil
		}
	}

	// Caught before the stash is made, so a Ctrl-C anywhere from here reaches
	// the undo below as a cancel -- and the deferred pop -- rather than
	// killing gg with the operation half done.
	intr := catchInterrupts()
	defer intr.stop()
	cleanup, err := handleDirtyTree(&p.cmdIO, "pull")
	if err != nil {
		return intr.or(err)
	}
	defer cleanup()

	if err := p.repo().Integrate(mode, upstream); err != nil {
		return p.undoIntegrate(currentBranch, mode, localCommit, intr.or(err))
	}

	// Render what landed in the same style as `gg diff` (coloured
	// compact-summary) rather than git's plain --stat, which Integrate
	// suppresses. Best-effort: a pull that succeeded is not failed by a
	// diff-render hiccup.
	if newCommit, err := p.repo().GetCommitHash(currentBranch); err == nil && newCommit != localCommit {
		if summary, derr := diffSummary(p.repo(), 0, localCommit+".."+newCommit); derr == nil && summary != "" {
			fmt.Fprintln(p.out(), summary)
		}
	}

	fmt.Fprintf(p.out(), "Pulled '%s' into '%s' (%s).\n", upstream, localRef(currentBranch), mode)
	return nil
}

// ensureUpstreamRef guarantees the remote-tracking ref named by upstream exists
// locally, so the commit lookups and divergence checks that follow have
// something to read.
//
// The fetch above just ran the remote's configured refspecs, so a still-missing
// tracking ref means one of exactly two things. Ask the remote which, rather
// than guessing -- the two want opposite responses:
//
//   - the branch is gone from the remote (or was never pushed): there is nothing
//     to pull and no fetch will conjure it. `gg doctor` already reports this as
//     gone-upstream and prescribes the fix, so just name it here.
//   - the branch is there: then the refspec cannot be covering it -- the
//     signature of a --single-branch clone, which `git clone --depth` implies.
//     Fetch that one ref explicitly, and say how to widen the refspec for good.
//     Leaving their config alone is deliberate; a pull shouldn't rewrite it.
func (p *PullCommand) ensureUpstreamRef(remote, remoteBranch, upstream string) error {
	if p.repo().RefExists(upstream) {
		return nil
	}

	// Reachability, not mere existence: an unreachable remote (auth, DNS) must
	// not be reported as a deleted branch.
	exists, reachable := p.repo().RemoteBranchReachability(remote, remoteBranch)
	if !reachable {
		return fmt.Errorf("no remote-tracking ref for '%s' and remote '%s' could not be queried", upstream, remote)
	}
	if !exists {
		return fmt.Errorf("nothing to pull: upstream '%s' does not exist on remote '%s'", upstream, remote)
	}

	fmt.Fprintf(p.err(),
		"%s remote '%s' has a narrow fetch refspec that does not cover '%s'; fetching it directly.\n"+
			"      widen it permanently with: git config remote.%s.fetch '+refs/heads/*:refs/remotes/%s/*'\n",
		paint(ansiBoldYellow, "note:"), remote, upstream, remote, remote)

	spec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", remoteBranch, upstream)
	if err := p.repo().Fetch(remote, spec); err != nil {
		return fmt.Errorf("fetching upstream ref '%s': %w", upstream, err)
	}
	return nil
}

// pullPR updates a checkout-pr branch to the latest PR head. It re-fetches the
// PR ref (which may have been force-pushed) and moves the branch to it:
//   - equal: already up to date.
//   - branch strictly behind: fast-forward (no local commits to lose).
//   - diverged (force-push, or local commits): the fetched head isn't a
//     descendant, so a move means discarding local work -- confirm first
//     (default no), matching `gg checkout-pr`'s reset-to-PR-state behaviour.
func (p *PullCommand) pullPR(branch string, m pr.Meta) error {
	if err := p.repo().Fetch(m.Remote, m.FetchRef(pr.ForgeOf(p.repo(), m.Remote))); err != nil {
		return err
	}

	local, err := p.repo().GetCommitHash(branch)
	if err != nil {
		return fmt.Errorf("getting local commit: %w", err)
	}
	fetched, err := p.repo().GetCommitHash("FETCH_HEAD")
	if err != nil {
		return fmt.Errorf("getting fetched PR commit: %w", err)
	}
	if local == fetched {
		fmt.Fprintf(p.out(), "Already up to date. '%s' matches PR #%d (%s).\n", localRef(branch), m.Number, m.Type)
		return nil
	}

	// Does the branch carry commits the fetched head lacks? If not, the fetched
	// head is a descendant and moving to it is a clean fast-forward.
	localAhead, err := p.repo().IsBranchAheadOfRemote(branch, "FETCH_HEAD")
	if err != nil {
		return fmt.Errorf("checking divergence: %w", err)
	}
	if localAhead {
		confirmed, err := p.sel().Confirm(
			fmt.Sprintf("'%s' has diverged from PR #%d (force-push or local commits). Reset it to the PR head, discarding local commits?", localRef(branch), m.Number),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(p.out(), "Left branch unchanged.")
			return nil
		}
	}

	cleanup, err := handleDirtyTree(&p.cmdIO, "pull")
	if err != nil {
		return err
	}
	defer cleanup()

	if err := p.repo().ResetHard("FETCH_HEAD"); err != nil {
		return fmt.Errorf("resetting to PR head: %w", err)
	}

	if summary, derr := diffSummary(p.repo(), 0, local+".."+fetched); derr == nil && summary != "" {
		fmt.Fprintln(p.out(), summary)
	}
	fmt.Fprintf(p.out(), "Updated '%s' to PR #%d (%s).\n", localRef(branch), m.Number, m.Type)
	return nil
}

// pullLeave is the mode selectMode hands back for the "leave as is" row. It is
// not a git.PullMode Integrate knows, so a caller that forgets to check for it
// gets an "unknown pull mode" error rather than a silent no-op.
const pullLeave git.PullMode = -1

// selectMode prompts for an integration strategy and maps the chosen label
// back to its PullMode. Labels come from PullMode.String(), so the picker and
// the mode stay in sync; the reset row names what it resets to and how much it
// drops, and is confirmed (default no) since it is the one choice that loses
// commits. Declining that confirmation is the same as picking "leave as is".
func (p *PullCommand) selectMode(currentBranch, upstream string, ahead, behind int) (git.PullMode, error) {
	labels := make([]string, 0, len(pullModes)+1)
	byLabel := make(map[string]git.PullMode, len(pullModes)+1)
	for _, m := range pullModes {
		label := m.String()
		if m == git.PullReset {
			label = fmt.Sprintf("reset to '%s' (drop %s)", upstream, localCommits(ahead))
		}
		labels = append(labels, label)
		byLabel[label] = m
	}
	labels = append(labels, pullLeaveLabel)
	byLabel[pullLeaveLabel] = pullLeave

	choice, err := p.sel().Select(
		fmt.Sprintf("Integrate '%s' into '%s' via", upstream, localRef(currentBranch)), labels)
	if err != nil {
		return 0, err
	}
	mode, ok := byLabel[choice]
	if !ok {
		return 0, fmt.Errorf("unknown integration mode: %s", choice)
	}
	if mode == git.PullReset {
		confirmed, err := p.sel().Confirm(
			fmt.Sprintf("Reset '%s' to '%s', dropping %s?", localRef(currentBranch), upstream, localCommits(ahead)), false)
		if err != nil {
			return 0, err
		}
		if !confirmed {
			return pullLeave, nil
		}
	}
	return mode, nil
}

// undoIntegrate puts the branch back after an integration that failed or was
// interrupted part-way, so a pull never leaves a half-done merge or rebase
// behind. What "back" takes depends on how far git got: an operation still in
// progress is aborted, which restores everything itself; otherwise a moved HEAD
// is reset (--keep, the user's changes were stashed before and are not git's to
// discard) and an unmoved one is left alone, as a refused fast-forward leaves
// it. The error that prompted the undo is handed through, annotated when the
// undo itself fails.
func (p *PullCommand) undoIntegrate(currentBranch string, mode git.PullMode, before string, cause error) error {
	if op, yes := p.repo().InProgress(); yes {
		var aerr error
		switch {
		case strings.Contains(op, "rebase"):
			aerr = p.repo().RebaseAbort()
		case strings.Contains(op, "merge"):
			aerr = p.repo().MergeAbort()
		default:
			return fmt.Errorf("%w; %s is in progress, finish or abort it by hand", cause, op)
		}
		if aerr != nil {
			return fmt.Errorf("%w; aborting the %s failed too (%v): finish or abort it by hand", cause, mode, aerr)
		}
		fmt.Fprintf(p.err(), "The %s stopped part-way; aborted it, so '%s' is as it was.\n", mode, localRef(currentBranch))
		return cause
	}
	now, err := p.repo().GetCommitHash(currentBranch)
	if err != nil || now == before {
		return cause
	}
	if err := p.repo().ResetKeep(before); err != nil {
		return fmt.Errorf("%w; putting '%s' back failed too (%v): restore it by hand with git reset --keep %s",
			cause, localRef(currentBranch), err, before)
	}
	fmt.Fprintf(p.err(), "The %s stopped part-way; put '%s' back as it was.\n", mode, localRef(currentBranch))
	return cause
}
