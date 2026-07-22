package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type PullCommand struct {
	cmdIO
}

// pullModes lists the integration strategies offered in the picker, in
// display order. ff-only leads: it's the safe default that refuses to create
// a merge commit or rewrite history behind your back.
var pullModes = []git.PullMode{git.PullFFOnly, git.PullRebase, git.PullMerge}

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
		return fmt.Errorf("no upstream configured for branch '%s'", currentBranch)
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
	if meta, ok, err := readPRMeta(p.repo(), currentBranch); err != nil {
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
	remote, _, ok := strings.Cut(upstream, "/")
	if !ok {
		return fmt.Errorf("unexpected upstream format: %s", upstream)
	}

	// Fetch first (empty refspec -> configured refspecs), so the divergence
	// check below reads the freshly-updated remote-tracking ref rather than a
	// stale cache. On a shallow clone this grabs new tip commits only.
	if err := p.repo().Fetch(remote, ""); err != nil {
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
		fmt.Fprintf(p.out(), "Already up to date. Local branch '%s' matches '%s'.\n", currentBranch, upstream)
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
	localAhead, err := p.repo().IsBranchAheadOfRemote(currentBranch, upstream)
	if err != nil {
		return fmt.Errorf("checking divergence: %w", err)
	}
	upstreamAhead, err := p.repo().IsBranchAheadOfRemote(upstream, currentBranch)
	if err != nil {
		return fmt.Errorf("checking divergence: %w", err)
	}
	if localAhead && !upstreamAhead {
		fmt.Fprintf(p.out(), "Nothing to pull. Local branch '%s' is ahead of '%s'.\n", currentBranch, upstream)
		return nil
	}

	mode := git.PullFFOnly // strictly behind: every mode fast-forwards.
	if localAhead {        // diverged: let the user pick how to reconcile.
		mode, err = p.selectMode(currentBranch, upstream)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return err
		}
	}

	cleanup, err := handleDirtyTree(&p.cmdIO, "pull")
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			return nil
		}
		return err
	}
	defer cleanup()

	if err := p.repo().Integrate(mode, upstream); err != nil {
		return err
	}

	// Render what landed in the same style as `gg diff` (coloured
	// compact-summary) rather than git's plain --stat, which Integrate
	// suppresses. Best-effort: a pull that succeeded is not failed by a
	// diff-render hiccup.
	if newCommit, err := p.repo().GetCommitHash(currentBranch); err == nil && newCommit != localCommit {
		if summary, derr := compactSummary(p.repo(), localCommit+".."+newCommit); derr == nil && summary != "" {
			fmt.Fprintln(p.out(), summary)
		}
	}

	fmt.Fprintf(p.out(), "Pulled '%s' into '%s' (%s).\n", upstream, currentBranch, mode)
	return nil
}

// pullPR updates a checkout-pr branch to the latest PR head. It re-fetches the
// PR ref (which may have been force-pushed) and moves the branch to it:
//   - equal: already up to date.
//   - branch strictly behind: fast-forward (no local commits to lose).
//   - diverged (force-push, or local commits): the fetched head isn't a
//     descendant, so a move means discarding local work -- confirm first
//     (default no), matching `gg checkout-pr`'s reset-to-PR-state behaviour.
func (p *PullCommand) pullPR(branch string, m prMeta) error {
	if err := p.repo().Fetch(m.remote, m.ref()); err != nil {
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
		fmt.Fprintf(p.out(), "Already up to date. Branch '%s' matches PR #%d (%s).\n", branch, m.number, m.typ)
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
			fmt.Sprintf("Branch '%s' has diverged from PR #%d (force-push or local commits). Reset it to the PR head, discarding local commits?", branch, m.number),
			false,
		)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			return err
		}
		if !confirmed {
			fmt.Fprintln(p.out(), "Left branch unchanged.")
			return nil
		}
	}

	cleanup, err := handleDirtyTree(&p.cmdIO, "pull")
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			return nil
		}
		return err
	}
	defer cleanup()

	if err := p.repo().ResetHard("FETCH_HEAD"); err != nil {
		return fmt.Errorf("resetting to PR head: %w", err)
	}

	if summary, derr := compactSummary(p.repo(), local+".."+fetched); derr == nil && summary != "" {
		fmt.Fprintln(p.out(), summary)
	}
	fmt.Fprintf(p.out(), "Updated '%s' to PR #%d (%s).\n", branch, m.number, m.typ)
	return nil
}

// selectMode prompts for an integration strategy and maps the chosen label
// back to its PullMode. Labels come straight from PullMode.String(), so the
// picker and the mode stay in sync.
func (p *PullCommand) selectMode(currentBranch, upstream string) (git.PullMode, error) {
	labels := make([]string, len(pullModes))
	byLabel := make(map[string]git.PullMode, len(pullModes))
	for i, m := range pullModes {
		labels[i] = m.String()
		byLabel[m.String()] = m
	}

	choice, err := p.sel().Select(
		fmt.Sprintf("Integrate '%s' into '%s' via", upstream, currentBranch), labels)
	if err != nil {
		return 0, err
	}
	mode, ok := byLabel[choice]
	if !ok {
		return 0, fmt.Errorf("unknown integration mode: %s", choice)
	}
	return mode, nil
}
