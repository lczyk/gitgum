package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/ui"
)

// planRemoteSelection resolves a picked remote branch into the action that
// lands on it. Either way it ends on a local branch tracking the remote, then
// hands off to the shared pull flow (pullCurrent) -- the same "fetch + offer to
// integrate" every switch selection ends with. When no local branch exists yet
// it offers to create a tracking one; when one does, it switches and re-points
// tracking at this remote if needed.
//
// Every question is asked here rather than inside the returned action, because
// declining one must not have cost the caller anything (see Execute: the
// dirty-tree answer is still unspent). That includes the fetch a question needs
// to be asked well: it updates remote-tracking refs only, nothing of the user's.
func (s *SwitchCommand) planRemoteSelection(remote, branch string) (func() error, error) {
	if !s.repo().BranchExists(branch) {
		confirmed, err := s.sel().Confirm(
			fmt.Sprintf("No '%s' yet. Create it tracking '%s/%s'?", localRef(branch), remote, branch), true)
		if err != nil {
			return nil, err
		}
		if !confirmed {
			fmt.Fprintln(s.err(), "Not creating a local tracking branch. Aborting switch.")
			return nil, ui.ErrCancelled
		}
		return func() error {
			return s.landOn(branch, s.createStep(branch, remote),
				fmt.Sprintf("Created and switched to '%s' tracking '%s/%s'.", localRef(branch), remote, branch))
		}, nil
	}

	trackingRemote, _ := s.repo().GetBranchTrackingRemote(branch)
	if trackingRemote == remote {
		return s.planCheckout(branch), nil
	}

	// The local branch tracks somewhere else (or nowhere). Re-pointing it at
	// this remote is right when the two are the same line of history, and a
	// question when they are not: a same-name branch on two remotes may be two
	// different things, and the retarget would make the pull reconcile them.
	// So read first -- fetch, measure -- and ask only when it matters.
	upstream := remote + "/" + branch
	if err := s.repo().Fetch(remote, ""); err != nil {
		return nil, err
	}
	if !s.repo().RefExists(upstream) {
		return nil, fmt.Errorf("no remote-tracking ref '%s' after fetching '%s'", upstream, remote)
	}
	ahead, behind, err := s.repo().AheadBehind(branch, upstream)
	if err != nil {
		return nil, fmt.Errorf("checking divergence: %w", err)
	}
	retarget := true
	if ahead > 0 && behind > 0 {
		was := "no upstream"
		if prev := s.currentUpstreamShort(branch); prev != "" {
			was = "'" + prev + "'"
		}
		fmt.Fprintf(s.out(), "'%s' tracks %s and has diverged from '%s' (ahead %d, behind %d).\n",
			localRef(branch), was, upstream, ahead, behind)
		retarget, err = s.sel().Confirm(fmt.Sprintf("Retarget tracking of '%s' to '%s'?", localRef(branch), upstream), false)
		if err != nil {
			return nil, err
		}
	}

	if !retarget {
		return func() error {
			return s.landOn(branch, s.checkoutStep(branch),
				fmt.Sprintf("Switched to '%s', tracking left as it was.", localRef(branch)))
		}, nil
	}
	return func() error {
		return s.landOn(branch, s.checkoutStep(branch),
			fmt.Sprintf("Switched to '%s'.", localRef(branch)), s.retargetTracking(remote, branch))
	}, nil
}

// currentUpstreamShort renders branch's upstream as "<remote>/<branch>" (or the
// bare branch for a local-tracking one), "" when it has none.
func (s *SwitchCommand) currentUpstreamShort(branch string) string {
	remote, remoteBranch, err := s.repo().GetBranchUpstream(branch)
	if err != nil || remote == "" {
		return ""
	}
	if remote == "." {
		return localRef(remoteBranch)
	}
	return remote + "/" + remoteBranch
}

// retargetTracking returns the landing step that points branch's upstream at
// remote/branch, paired with its undo: a function that puts the previous
// upstream back (or unsets it, if there was none) and hands through the error
// that prompted it, annotated when the restore itself fails.
//
// The previous upstream is restored from the raw branch.<name>.remote /
// .merge pair rather than via --set-upstream-to, which refuses an upstream
// whose remote-tracking ref is absent locally (a narrow fetch refspec) --
// exactly the config that was there a moment ago.
func (s *SwitchCommand) retargetTracking(remote, branch string) landingStep {
	var prevRemote, prevMerge string
	return landingStep{
		do: func() error {
			var err error
			if prevRemote, err = s.repo().BranchConfigGet(branch, "remote"); err != nil {
				return err
			}
			if prevMerge, err = s.repo().BranchConfigGet(branch, "merge"); err != nil {
				return err
			}
			if _, stderr, err := s.repo().RunWrite("branch", "--set-upstream-to="+remote+"/"+branch, branch); err != nil {
				return fmt.Errorf("setting tracking reference: %w: %s", err, stderr)
			}
			fmt.Fprintf(s.out(), "Set tracking of '%s' to '%s/%s'.\n", localRef(branch), remote, branch)
			return nil
		},
		undo: func(cause error) error {
			var (
				rerr error
				was  string
			)
			if prevRemote == "" || prevMerge == "" {
				was = "no upstream"
				_, stderr, err := s.repo().RunWrite("branch", "--unset-upstream", branch)
				if err != nil {
					rerr = fmt.Errorf("%w: %s", err, stderr)
				}
			} else {
				was = "'" + upstreamShort(prevRemote, prevMerge) + "'"
				if err := s.repo().BranchConfigSet(branch, "remote", prevRemote); err != nil {
					rerr = err
				} else if err := s.repo().BranchConfigSet(branch, "merge", prevMerge); err != nil {
					rerr = err
				}
			}
			if rerr != nil {
				return fmt.Errorf("%w; restoring the tracking of '%s' to %s failed too (%v)",
					cause, localRef(branch), was, rerr)
			}
			fmt.Fprintf(s.err(), "Restored the tracking of '%s' to %s.\n", localRef(branch), was)
			return cause
		},
	}
}

// upstreamShort renders a branch.<name>.remote / .merge pair the way gg names
// branches: "<remote>/<branch>", or "local/<branch>" for the local remote ".".
func upstreamShort(remote, merge string) string {
	branch := strings.TrimPrefix(merge, "refs/heads/")
	if remote == "." {
		return localRef(branch)
	}
	return remote + "/" + branch
}
