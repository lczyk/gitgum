package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type SwitchCommand struct {
	cmdIO
}

// resolveCurrentBranchContext figures out the current branch, its tracking
// remote, and a human-readable status line for display. In detached HEAD,
// returns currentBranch="" so downstream branch filters don't match the literal
// string "HEAD" (which can't be a real branch but is what rev-parse returns).
func resolveCurrentBranchContext(r git.Repo) (currentBranch, trackingRemote, statusLine string, err error) {
	currentBranch, err = r.GetCurrentBranch()
	if err != nil {
		return "", "", "", fmt.Errorf("getting current branch: %w", err)
	}

	// "HEAD" from rev-parse --abbrev-ref means detached HEAD. Skip upstream
	// lookup since HEAD@{u} fails with "HEAD does not point to a branch".
	if currentBranch == "HEAD" {
		return "", "", "Currently in detached HEAD state.", nil
	}

	trackingRemote, err = r.GetBranchTrackingRemote(currentBranch)
	if err != nil {
		return "", "", "", fmt.Errorf("getting tracking remote: %w", err)
	}

	branchDisplay := currentBranch
	switch trackingRemote {
	case "":
	case ".": // tracks another local branch: "local" is the remote gg would name
		branchDisplay = fmt.Sprintf("(local/)%s", currentBranch)
	default:
		branchDisplay = fmt.Sprintf("(%s/)%s", trackingRemote, currentBranch)
	}
	return currentBranch, trackingRemote, "Current branch is: " + branchDisplay, nil
}

// detachedShortSHA returns the short sha HEAD is detached at, or "" when HEAD
// is on a branch (currentBranch != "", per resolveCurrentBranchContext) or when
// there is no commit to name yet.
func detachedShortSHA(r git.Repo, currentBranch string) string {
	if currentBranch != "" {
		return ""
	}
	short, _, err := r.Run("rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(short)
}

func (s *SwitchCommand) checkoutBranch(branch string) error {
	if err := s.repo().Checkout(branch); err != nil {
		return fmt.Errorf("could not switch to '%s': %w", localRef(branch), err)
	}
	return nil
}

// landingStep is one change a landing makes between the checkout and the
// pull, paired with the undo that takes it back. undo receives the error that
// prompted it and returns what the caller should report -- usually that same
// error, annotated when the undo itself failed. A nil undo has nothing to take
// back.
type landingStep struct {
	do   func() error
	undo func(cause error) error
}

// checkoutStep lands on an existing branch. Nothing to undo beyond going back,
// which landOn does itself.
func (s *SwitchCommand) checkoutStep(branch string) landingStep {
	return landingStep{do: func() error { return s.checkoutBranch(branch) }}
}

// createStep lands on a new branch cut from a remote one, tracking it. Its undo
// deletes the branch again -- but only while it still sits on the remote tip
// it was cut from, so nothing a failed pull may have left on it is lost.
func (s *SwitchCommand) createStep(branch, remote string) landingStep {
	start := remote + "/" + branch
	return landingStep{
		do: func() error {
			if err := s.repo().CheckoutNewBranch(branch, start); err != nil {
				return fmt.Errorf("creating tracking branch: %w", err)
			}
			return nil
		},
		undo: func(cause error) error {
			tip, err := s.repo().GetCommitHash(branch)
			if err != nil {
				return cause
			}
			if startTip, err := s.repo().GetCommitHash(start); err != nil || tip != startTip {
				return cause
			}
			if _, stderr, err := s.repo().RunWrite("branch", "-D", branch); err != nil {
				return fmt.Errorf("%w; deleting '%s' again failed too (%v): %s", cause, localRef(branch), err, strings.TrimSpace(stderr))
			}
			fmt.Fprintf(s.err(), "Deleted '%s' again.\n", localRef(branch))
			return cause
		},
	}
}

// landOn is how every switch selection lands: check the branch out, apply the
// steps, say so, pull. Anything that fails or is cancelled from the checkout on
// is taken back in reverse -- the steps' undos, then the checkout (back to the
// branch, or the commit, you were on), then the checkout step's own undo -- so a
// switch that did not reach its goal leaves you where you started, with the
// failure or cancel reported as such. A Ctrl-C mid-git is caught for the same
// reason: gg lives to take its steps back, and reports a cancel.
//
// Only the pull's own decline (nothing to pull, or "leave as is") is not a
// failure here: the branch is reached, and that was the goal.
func (s *SwitchCommand) landOn(branch string, checkout landingStep, switched string, steps ...landingStep) error {
	from, err := s.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	fromName := localRef(from)
	if from == "HEAD" { // detached: go back to the commit, not to a branch named HEAD
		fromName = detachedShortSHA(s.repo(), "")
		if from, err = s.repo().GetCommitHash("HEAD"); err != nil {
			return fmt.Errorf("getting current commit: %w", err)
		}
	}

	intr := catchInterrupts()
	defer intr.stop()

	if err := checkout.do(); err != nil {
		return intr.or(err)
	}
	var undos []func(error) error
	back := func(cause error) error {
		cause = intr.or(cause)
		for i := len(undos) - 1; i >= 0; i-- {
			cause = undos[i](cause)
		}
		if from != branch {
			if cerr := s.repo().Checkout(from); cerr != nil {
				return fmt.Errorf("%w; going back to '%s' failed too (%v)", cause, fromName, cerr)
			}
			fmt.Fprintf(s.err(), "Back on '%s'.\n", fromName)
		}
		if checkout.undo != nil {
			cause = checkout.undo(cause)
		}
		return cause
	}

	for _, step := range steps {
		if err := step.do(); err != nil {
			return back(err)
		}
		if step.undo != nil {
			undos = append(undos, step.undo)
		}
	}
	fmt.Fprintln(s.out(), switched)
	if err := s.pullCurrent(); err != nil {
		return back(err)
	}
	return nil
}

func (s *SwitchCommand) Execute(args []string) error {
	r := s.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	// Pre-picker reads, overlapped -- see runConcurrent.
	var (
		currentBranch, trackingRemote, statusLine string
		remotes                                   []string
		dirty                                     []git.Entry
	)
	if err := runConcurrent(
		func() (err error) {
			currentBranch, trackingRemote, statusLine, err = resolveCurrentBranchContext(r)
			return err
		},
		func() (err error) {
			remotes, err = r.GetRemotes()
			if err != nil {
				return fmt.Errorf("getting remotes: %w", err)
			}
			return nil
		},
		func() (err error) {
			dirty, err = r.DirtyTracked()
			return err
		},
	); err != nil {
		return err
	}
	fmt.Fprintln(s.out(), statusLine)

	// Asked here, where the listing is what you are reading, but applied only
	// once a branch is picked -- backing out of the picker must leave a
	// discard unperformed.
	applyDirty, err := decideDirtyEntries(&s.cmdIO, "switch", dirty)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// includeCurrent + pickCurrent: the current branch shows in the list and
	// picking it pulls it. Keeping it visible means the list isn't missing the
	// branch you're staring at in the status line above. A detached HEAD has
	// no such branch, so detachedAt stands in for it.
	src := streamBranches(ctx, r, s.err(), currentBranch, trackingRemote, remotes,
		branchStreamOpts{includeCurrent: true, markCheckedOut: true, pickCurrent: true, detachedAt: detachedShortSHA(r, currentBranch)})

	id, err := s.sel().SelectStream(ctx, "Select a branch to switch to", src.Feed)
	cancel()
	if err != nil {
		// Only a cancel is "nothing selected"; a picker that actually failed
		// gets its own error rather than a message implying the user backed out.
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(s.err(), "No branch selected. Aborting switch.")
			return err
		}
		return err
	}
	picked, err := src.pick(id)
	if err != nil {
		return err
	}

	// Planned before the dirty answer is spent: a remote branch with no local
	// counterpart still has a question to ask, and declining it cancels the
	// switch.
	act, err := s.planSelection(picked)
	if err != nil {
		return err
	}

	// Caught from the stash on, not from the landing: a Ctrl-C between the two
	// would otherwise kill gg with the stash made and its pop never run.
	intr := catchInterrupts()
	defer intr.stop()
	cleanup, err := applyDirty()
	if err != nil {
		return intr.or(err)
	}
	defer cleanup()

	return intr.or(act())
}

// planSelection resolves what the picked entry means and returns the action
// that carries it out. Nothing it does touches the repo -- any question left to
// ask is asked here, so a caller can hold back an irreversible step until the
// switch is certain to happen.
//
// Every landing ends the same way: be on the branch, then bring it up to date
// via the shared pull flow (fetch + offer to integrate, PR-aware). The HEAD row
// is just the case where the checkout is a no-op -- you're already there -- so
// it goes straight to the update.
func (s *SwitchCommand) planSelection(p branchPick) (func() error, error) {
	if p.current {
		return s.pullCurrent, nil
	}

	switch p.kind {
	case localBranch, trackedBranch:
		return s.planCheckout(p.branch), nil
	case remoteBranch:
		return s.planRemoteSelection(p.remote, p.branch)
	default:
		return nil, fmt.Errorf("cannot switch to %s: not a branch", p.ref())
	}
}

func (s *SwitchCommand) planCheckout(branch string) func() error {
	return func() error {
		return s.landOn(branch, s.checkoutStep(branch), fmt.Sprintf("Switched to '%s'.", localRef(branch)))
	}
}

// applySelection plans and acts in one step, for callers holding nothing back.
func (s *SwitchCommand) applySelection(p branchPick) error {
	act, err := s.planSelection(p)
	if err != nil {
		return err
	}
	return act()
}

// pullCurrent updates the branch currently checked out via the shared pull
// flow. A branch with no upstream (a purely local branch, never pushed) isn't
// an error here -- there's simply nothing to pull -- so errNoUpstream is
// reported and swallowed. PR branches and branches with an upstream integrate
// as `gg pull` would.
func (s *SwitchCommand) pullCurrent() error {
	branch, err := s.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	pull := &PullCommand{cmdIO: s.cmdIO}
	if err := pull.pullBranch(branch); err != nil {
		if errors.Is(err, errNoUpstream) {
			fmt.Fprintf(s.out(), "'%s' has no upstream; nothing to pull.\n", localRef(branch))
			return nil
		}
		return err
	}
	return nil
}
