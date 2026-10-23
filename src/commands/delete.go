package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

type DeleteCommand struct {
	cmdIO
}

func (d *DeleteCommand) Execute(args []string) error {
	r := d.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	// Pre-picker reads, overlapped -- see runConcurrent.
	//
	// The empty-repo guard runs first so its error wins: on a genuinely-empty
	// repo (no commits yet) resolveCurrentBranchContext's rev-parse HEAD fails
	// with a cryptic message, but runConcurrent returns errors in fn order, so
	// the clear "no local branches found" takes precedence. NOTE: a repo with
	// zero local branches but existing remote branches isn't caught -- in a
	// normal working repo you always have at least one local branch, so this
	// only fires on a fresh init.
	var (
		currentBranch, trackingRemote string
		remotes                       []string
		locals                        []git.LocalBranch
	)
	if err := runConcurrent(
		func() (err error) {
			if locals, err = r.LocalBranches(); err != nil {
				return fmt.Errorf("getting local branches: %w", err)
			}
			if len(locals) == 0 {
				return fmt.Errorf("no local branches found")
			}
			return nil
		},
		func() (err error) {
			currentBranch, trackingRemote, _, err = resolveCurrentBranchContext(r)
			return err
		},
		func() (err error) {
			remotes, err = r.GetRemotes()
			if err != nil {
				return fmt.Errorf("getting remotes: %w", err)
			}
			return nil
		},
	); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// includeCurrent: current branch shows but is unselectable (checked-out
	// marker), same picker as switch. This lists remote branches too, so a
	// remote-only branch is deletable -- the whole point of the unification.
	src := streamBranches(ctx, r, d.err(), currentBranch, trackingRemote, remotes,
		branchStreamOpts{includeCurrent: true, markCheckedOut: true, locals: locals})

	id, err := d.sel().SelectStream(ctx, "Select a branch to delete", src.Feed)
	cancel()
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return err
		}
		return err
	}
	picked, err := src.pick(id)
	if err != nil {
		return err
	}

	return d.applyDeletion(picked)
}

// applyDeletion dispatches the picked row (mirrors switch's applySelection):
// local branches go through the safe->force local flow, remote entries are
// deleted straight off the remote.
func (d *DeleteCommand) applyDeletion(p branchPick) error {
	intr := catchInterrupts()
	defer intr.stop()
	return intr.or(d.deleteEntry(p))
}

func (d *DeleteCommand) deleteEntry(p branchPick) error {
	switch p.kind {
	case localBranch, trackedBranch:
		return d.deleteLocal(p.branch)
	case remoteBranch:
		return d.deleteRemoteOnly(p.remote, p.branch)
	default:
		return fmt.Errorf("cannot delete %s: not a branch", p.ref())
	}
}

func (d *DeleteCommand) deleteLocal(branch string) error {
	// main/master deletion is dangerous enough to warrant a confirmation
	if branch == "main" || branch == "master" {
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("You are about to delete '%s'. This is usually the main branch of the repository. Are you sure you want to proceed?", localRef(branch)),
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

	// non-fatal: skip remote deletion if upstream lookup fails
	remoteName, remoteBranchName, _ := d.repo().GetBranchUpstream(branch)

	needsToDeleteRemote := false

	if remoteName != "" && remoteBranchName != "" {
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("'%s' is tracking '%s/%s'. Do you want to delete the remote branch as well?", localRef(branch), remoteName, remoteBranchName),
			false,
		)
		if err != nil {
			return err
		}
		needsToDeleteRemote = confirmed
	}

	// Whether the branch is merged is a fact, readable without touching
	// anything, so the force question joins the others up front instead of
	// arriving after an attempt that had to fail to be informative. Every
	// question is now asked before anything is deleted.
	force := !d.repo().BranchMerged(branch)
	if force {
		what := "the local branch"
		if needsToDeleteRemote {
			what = "the local branch and the remote branch"
		}
		confirmed, err := d.sel().Confirm(
			fmt.Sprintf("'%s' is not fully merged. Do you want to force delete %s?", localRef(branch), what), false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}
	}

	flag := "-d"
	if force {
		flag = "-D"
	}
	if _, stderr, err := d.repo().RunWrite("branch", flag, branch); err != nil {
		return fmt.Errorf("deleting '%s': %w: %s", localRef(branch), err, strings.TrimSpace(stderr))
	}
	if force {
		fmt.Fprintf(d.out(), "Force deleted '%s'.\n", localRef(branch))
	} else {
		fmt.Fprintf(d.out(), "Deleted '%s'.\n", localRef(branch))
	}

	if needsToDeleteRemote {
		if err := d.repo().RunWriteStream("push", "--delete", remoteName, remoteBranchName); err != nil {
			return fmt.Errorf("deleting remote branch: %w", err)
		}
		fmt.Fprintf(d.out(), "Deleted remote branch '%s/%s'.\n", remoteName, remoteBranchName)
	}

	return nil
}

// deleteRemoteOnly deletes a branch that exists only on the remote (no local
// counterpart selected), e.g. a leftover branch after the local copy is gone.
func (d *DeleteCommand) deleteRemoteOnly(remote, branch string) error {
	confirmed, err := d.sel().Confirm(
		fmt.Sprintf("Delete remote branch '%s/%s'?", remote, branch),
		false,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(d.out(), "Aborting delete.")
		return nil
	}

	if err := d.repo().RunWriteStream("push", "--delete", remote, branch); err != nil {
		return fmt.Errorf("deleting remote branch: %w", err)
	}
	fmt.Fprintf(d.out(), "Deleted remote branch '%s/%s'.\n", remote, branch)
	return nil
}
