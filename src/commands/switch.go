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

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext(r)
	if err != nil {
		return err
	}
	fmt.Fprintln(s.out(), statusLine)

	cleanup, err := handleDirtyTree(&s.cmdIO, "switch")
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			fmt.Fprintln(s.out(), "Aborted.")
			return nil
		}
		return err
	}
	defer cleanup()

	remotes, err := r.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// includeCurrent: the current branch shows in the list but is unselectable
	// -- it carries the checked-out marker for this worktree, so
	// isCheckedOutElsewhere blocks it. Keeping it visible means the list isn't
	// missing the branch you're staring at in the status line above.
	src := streamBranches(ctx, r, s.err(), currentBranch, trackingRemote, remotes,
		branchStreamOpts{includeCurrent: true, markCheckedOut: true})

	selected, err := s.sel().SelectStream(ctx, "Select a branch to switch to", src, isCheckedOutElsewhere)
	cancel()
	if err != nil {
		fmt.Fprintln(s.err(), "No branch selected. Aborting switch.")
		if errors.Is(err, ui.ErrCancelled) {
			return nil
		}
		return err
	}

	if err := s.applySelection(selected); err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return nil
		}
		return err
	}
	return nil
}

func (s *SwitchCommand) applySelection(selected string) error {
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
		return nil
	case "local/remote":
		branch, ok := localRemoteBranch(name)
		if !ok {
			return fmt.Errorf("invalid local/remote branch format: %s", name)
		}
		if err := s.checkoutBranch(branch); err != nil {
			return err
		}
		fmt.Fprintf(s.out(), "Switched to branch '%s'.\n", branch)
		s.warnIfRemoteUnreachable(branch)
		return nil
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
