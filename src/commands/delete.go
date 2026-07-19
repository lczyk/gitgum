package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

	// The picker can't open until these read-only queries return, so run them
	// concurrently -- see runConcurrent.
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
	)
	if err := runConcurrent(
		func() error {
			locals, err := r.GetLocalBranches()
			if err != nil {
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
		branchStreamOpts{includeCurrent: true, markCheckedOut: true})

	selected, err := d.sel().SelectStream(ctx, "Select a branch to delete", src, isUnselectable)
	cancel()
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			fmt.Fprintln(d.out(), "Aborting delete.")
			return nil
		}
		return err
	}

	return d.applyDeletion(selected)
}

// applyDeletion parses the typed picker entry (mirrors switch's applySelection)
// and dispatches: local branches go through the safe->force local flow, remote
// entries are deleted straight off the remote.
func (d *DeleteCommand) applyDeletion(selected string) error {
	typ, name, err := parseBranchEntry(selected)
	if err != nil {
		return err
	}

	switch typ {
	case "local":
		return d.deleteLocal(name)
	case "local/remote":
		branch, ok := localRemoteBranch(name)
		if !ok {
			return fmt.Errorf("invalid local/remote branch format: %s", name)
		}
		return d.deleteLocal(branch)
	case "remote":
		remoteParts := strings.SplitN(name, "/", 2)
		if len(remoteParts) != 2 {
			return fmt.Errorf("invalid remote branch format: %s", name)
		}
		return d.deleteRemoteOnly(remoteParts[0], remoteParts[1])
	default:
		return fmt.Errorf("unknown branch type: %s", typ)
	}
}

func (d *DeleteCommand) deleteLocal(branch string) error {
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
	_, _, err := d.repo().RunWrite("branch", "-d", branch)
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

	if _, stderr, err := d.repo().RunWriteStream("push", "--delete", remote, branch); err != nil {
		return fmt.Errorf("deleting remote branch: %w: %s", err, strings.TrimSpace(stderr))
	}
	fmt.Fprintf(d.out(), "Deleted remote branch '%s/%s'.\n", remote, branch)
	return nil
}
