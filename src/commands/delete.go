package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/ui"
)

type DeleteCommand struct {
	cmdIO
}

func (d *DeleteCommand) Execute(args []string) error {
	if err := d.repo().CheckInRepo(); err != nil {
		return err
	}

	branches, err := d.repo().GetLocalBranches()
	if err != nil {
		return fmt.Errorf("getting local branches: %w", err)
	}

	if len(branches) == 0 {
		return fmt.Errorf("no local branches found")
	}

	branch, err := d.sel().Select("Select a branch to delete", branches)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}
		return err
	}

	// main/master deletion is dangerous enough to warrant a confirmation
	if branch == "main" || branch == "master" {
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("You are about to delete the '%s' branch. This is usually the main branch of the repository. Are you sure you want to proceed?", branch),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}
	}

	// if user wants to delete their current branch, offer to switch first
	currentBranch, err := d.repo().GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if branch == currentBranch {
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("You are currently on branch '%s'. Do you want to switch to another branch before deleting it?", branch),
			true,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}

		var otherBranches []string
		for _, b := range branches {
			if b != branch {
				otherBranches = append(otherBranches, b)
			}
		}

		if len(otherBranches) == 0 {
			return fmt.Errorf("no other branches to switch to")
		}

		otherBranch, err := d.sel().Select("Select a branch to switch to", otherBranches)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				fmt.Fprintln(d.out(), "Aborting delete.")
				return nil
			}
			return err
		}

		if _, stderr, err := d.repo().RunWriteStream("checkout", otherBranch); err != nil {
			return fmt.Errorf("switching to branch '%s': %w: %s", otherBranch, err, strings.TrimSpace(stderr))
		}
		fmt.Fprintf(d.out(), "Switched to branch '%s'.\n", otherBranch)
	}

	// non-fatal: skip remote deletion if upstream lookup fails
	remoteName, remoteBranchName, _ := d.repo().GetBranchUpstream(branch)

	needsToDeleteRemote := false

	if remoteName != "" && remoteBranchName != "" {
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("Branch '%s' is tracking remote branch '%s/%s'. Do you want to delete the remote branch as well?", branch, remoteName, remoteBranchName),
			false,
		)
		if err != nil {
			return err
		}
		needsToDeleteRemote = confirmed
	}

	// try safe delete first, fall back to force delete with confirmation
	_, _, err = d.repo().RunWrite("branch", "-d", branch)
	if err != nil {
		var confirmMsg string
		if needsToDeleteRemote {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch and the remote branch?", branch)
		} else {
			confirmMsg = fmt.Sprintf("Branch '%s' is not fully merged. Do you want to force delete the local branch?", branch)
		}

		confirmed, err := d.sel().Confirm(confirmMsg, false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}

		if _, stderr, err := d.repo().RunWrite("branch", "-D", branch); err != nil {
			return fmt.Errorf("force deleting branch '%s': %w: %s", branch, err, strings.TrimSpace(stderr))
		}
		fmt.Fprintf(d.out(), "Force deleted local branch '%s'.\n", branch)
	} else {
		fmt.Fprintf(d.out(), "Deleted local branch '%s'.\n", branch)
	}

	if needsToDeleteRemote {
		if _, stderr, err := d.repo().RunWriteStream("push", "--delete", remoteName, remoteBranchName); err != nil {
			return fmt.Errorf("deleting remote branch: %w: %s", err, strings.TrimSpace(stderr))
		}
		fmt.Fprintf(d.out(), "Deleted remote branch '%s/%s'.\n", remoteName, remoteBranchName)
	}

	return nil
}
