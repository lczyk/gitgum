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
	if trackingRemote != "" {
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
		return fmt.Errorf("could not switch to branch '%s': %w", branch, err)
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
		dirty                                     []string
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
			dirty, err = r.DirtyTrackedLines()
			return err
		},
	); err != nil {
		return err
	}
	fmt.Fprintln(s.out(), statusLine)

	cleanup, err := handleDirtyLines(&s.cmdIO, "switch", dirty)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// includeCurrent: the current branch shows in the list but is unselectable
	// -- it carries the checked-out marker for this worktree, so
	// isUnselectable blocks it. Keeping it visible means the list isn't
	// missing the branch you're staring at in the status line above. A detached
	// HEAD has no such branch, so detachedAt stands in for it.
	src := streamBranches(ctx, r, s.err(), currentBranch, trackingRemote, remotes,
		branchStreamOpts{includeCurrent: true, markCheckedOut: true, detachedAt: detachedShortSHA(r, currentBranch)})

	selected, err := s.sel().SelectStream(ctx, "Select a branch to switch to", src, switchUnselectable)
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

	if err := s.applySelection(selected); err != nil {
		return err
	}
	return nil
}

func (s *SwitchCommand) applySelection(selected string) error {
	// Every landing ends the same way: be on the branch, then bring it up to
	// date via the shared pull flow (fetch + offer to integrate, PR-aware). The
	// HEAD row is just the case where the checkout is a no-op -- you're already
	// there -- so it goes straight to the update.
	if strings.Contains(selected, currentBranchMarker) {
		return s.pullCurrent()
	}

	typ, name, err := parseBranchEntry(selected)
	if err != nil {
		return err
	}

	switch typ {
	case "local":
		if err := s.checkoutBranch(name); err != nil {
			return err
		}
		fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", name)
		return s.pullCurrent()
	case "local/remote":
		branch, ok := localRemoteBranch(name)
		if !ok {
			return fmt.Errorf("invalid local/remote branch format: %s", name)
		}
		if err := s.checkoutBranch(branch); err != nil {
			return err
		}
		fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", branch)
		return s.pullCurrent()
	case "remote":
		remoteParts := strings.SplitN(name, "/", 2)
		if len(remoteParts) != 2 {
			return fmt.Errorf("invalid remote branch format: %s", name)
		}
		return s.handleRemoteSelection(remoteParts[0], remoteParts[1])
	default:
		return fmt.Errorf("unknown branch type: %s", typ)
	}
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
			fmt.Fprintf(s.out(), "Branch '%s' has no upstream; nothing to pull.\n", branch)
			return nil
		}
		return err
	}
	return nil
}
