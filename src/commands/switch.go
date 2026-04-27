package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/lczyk/gitgum/internal/cmdrun"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

type SwitchCommand struct{}

// resolveCurrentBranchContext figures out the current branch, its tracking
// remote, and a human-readable status line for display. In detached HEAD,
// returns currentBranch="" so downstream branch filters don't match the literal
// string "HEAD" (which can't be a real branch but is what rev-parse returns).
func resolveCurrentBranchContext() (currentBranch, trackingRemote, statusLine string, err error) {
	currentBranch, err = git.GetCurrentBranch()
	if err != nil {
		return "", "", "", fmt.Errorf("getting current branch: %w", err)
	}

	// "HEAD" from rev-parse --abbrev-ref means detached HEAD. Skip upstream
	// lookup since HEAD@{u} fails with "HEAD does not point to a branch".
	if currentBranch == "HEAD" {
		return "", "", "Currently in detached HEAD state.", nil
	}

	trackingRemote, err = git.GetBranchTrackingRemote(currentBranch)
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
	_, stderr, err := cmdrun.Run("git", "checkout", "--quiet", branch)
	if err != nil {
		return fmt.Errorf("could not switch to branch '%s': %s", branch, stderr)
	}
	return nil
}

func (s *SwitchCommand) Execute(args []string) error {
	if err := git.CheckInRepo(); err != nil {
		return err
	}

	currentBranch, trackingRemote, statusLine, err := resolveCurrentBranchContext()
	if err != nil {
		return err
	}
	fmt.Println(statusLine)

	dirty, err := git.IsDirty()
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "You have local changes that would be overwritten by switching branches. Please commit or stash them before switching.")
		return fmt.Errorf("local changes would be overwritten")
	}

	remotes, err := git.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	branches, lock := streamBranches(ctx, currentBranch, trackingRemote, remotes)

	selected, err := pickBranch(ctx, branches, lock)
	cancel()
	if err != nil {
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

func pickBranch(ctx context.Context, branches *[]string, lock *sync.Mutex) (string, error) {
	prompt := "Select a branch to switch to"
	idxs, err := fuzzyfinder.Find(ctx, branches, lock, fuzzyfinder.Opt{
		Prompt:  prompt + ": ",
		Height:  10,
		Reverse: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "No branch selected. Aborting switch.")
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return "", ui.ErrCancelled
		}
		return "", err
	}

	lock.Lock()
	defer lock.Unlock()
	idx := idxs[0]
	if idx < 0 || idx >= len(*branches) {
		return "", fmt.Errorf("invalid branch selection index: %d", idx)
	}
	return (*branches)[idx], nil
}

func (s *SwitchCommand) applySelection(selected string) error {
	parts := strings.SplitN(selected, ": ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid selection: %s", selected)
	}
	typ, name := parts[0], parts[1]

	switch typ {
	case "local", "local/remote":
		if err := s.checkoutBranch(name); err != nil {
			return err
		}
		fmt.Printf("Switched to branch '%s'.\n", name)
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
