package commands

import (
	"errors"
	"fmt"
)

// CheckoutPRCommand is the interactive dispatcher: pick a remote, pick a PR,
// check it out. The pure PR-ref listing/parsing it drives lives in pr.go.
type CheckoutPRCommand struct {
	cmdIO
}

func (c *CheckoutPRCommand) Execute(args []string) error {
	if err := c.repo().CheckInRepo(); err != nil {
		return err
	}

	remotes, err := c.repo().GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(c.err(), "No remotes found. Aborting checkout-pr.")
		return fmt.Errorf("no remotes")
	}

	var remote string
	if len(remotes) == 1 {
		remote = remotes[0] // only one remote: no point asking.
	} else {
		remote, err = c.sel().Select("Select a remote to fetch PR from", remotes)
		if err != nil {
			fmt.Fprintln(c.err(), "No remote selected. Aborting checkout-pr.")
			return err
		}
	}

	prRefs, err := c.getPRRefs(remote)
	if err != nil {
		return fmt.Errorf("getting PR refs: %w", err)
	}

	if len(prRefs) == 0 {
		fmt.Fprintln(c.err(), "No pull requests found on remote. Aborting checkout-pr.")
		return fmt.Errorf("no pull requests found")
	}

	prOptions := formatPROptions(prRefs)

	selected, err := c.sel().Select("Select a pull request to checkout", prOptions)
	if err != nil {
		fmt.Fprintln(c.err(), "No PR selected. Aborting checkout-pr.")
		return err
	}

	prNumber, prType, err := parsePRSelection(selected)
	if err != nil {
		return err
	}

	return c.checkoutPR(remote, prNumber, prType)
}

func (c *CheckoutPRCommand) getPRRefs(remote string) ([]PRRef, error) {
	fmt.Fprintln(c.out(), "Fetching pull request references from remote:", remote)
	stdout, err := c.repo().LsRemote(remote)
	if err != nil {
		return nil, fmt.Errorf("listing remote refs: %w", err)
	}

	return parsePRRefs(stdout), nil
}

func (c *CheckoutPRCommand) checkoutPR(remote string, prNumber int, prType string) error {
	meta := prMeta{remote: remote, number: prNumber, typ: prType}
	branchName := prBranchName(remote, prNumber)
	prRef := meta.ref()

	if c.repo().BranchExists(branchName) {
		confirmed, err := c.sel().Confirm(
			fmt.Sprintf("Branch '%s' already exists. Reset it to the latest PR state?", branchName),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			if err := c.repo().Checkout(branchName); err != nil {
				return fmt.Errorf("checking out existing branch '%s': %w", branchName, err)
			}
			// Backfill metadata in case the branch predates it, so a later
			// `gg pull` can still update the PR.
			if err := writePRMeta(c.repo(), branchName, meta); err != nil {
				return fmt.Errorf("recording PR metadata: %w", err)
			}
			fmt.Fprintf(c.out(), "Switched to existing branch '%s'.\n", branchName)
			return nil
		}

		// The reset below is destructive, so the working tree has to be dealt
		// with first -- same guard the fresh-branch path and `gg pull`'s PR
		// flow use, rather than letting reset --hard eat uncommitted work.
		cleanup, err := handleDirtyTree(&c.cmdIO, "checkout-pr")
		if err != nil {
			if errors.Is(err, errDirtyTreeAborted) {
				fmt.Fprintln(c.out(), "Aborted.")
				return nil
			}
			return err
		}
		defer cleanup()

		fmt.Fprintf(c.out(), "Fetching PR #%d from %s...\n", prNumber, remote)
		if err := c.repo().Fetch(remote, prRef); err != nil {
			return fmt.Errorf("fetching PR: %w", err)
		}

		if err := c.repo().Checkout(branchName); err != nil {
			return fmt.Errorf("checking out branch '%s': %w", branchName, err)
		}

		if err := c.repo().ResetHard("FETCH_HEAD"); err != nil {
			return fmt.Errorf("resetting branch: %w", err)
		}

		if err := writePRMeta(c.repo(), branchName, meta); err != nil {
			return fmt.Errorf("recording PR metadata: %w", err)
		}

		fmt.Fprintf(c.out(), "Reset branch '%s' to PR #%d (%s).\n", branchName, prNumber, prType)
		return nil
	}

	cleanup, err := handleDirtyTree(&c.cmdIO, "checkout-pr")
	if err != nil {
		if errors.Is(err, errDirtyTreeAborted) {
			fmt.Fprintln(c.out(), "Aborted.")
			return nil
		}
		return err
	}
	defer cleanup()

	fmt.Fprintf(c.out(), "Fetching PR #%d from %s...\n", prNumber, remote)
	if err := c.repo().Fetch(remote, prRef); err != nil {
		return fmt.Errorf("fetching PR: %w", err)
	}

	if err := c.repo().CheckoutNewBranch(branchName, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("creating and checking out branch: %w", err)
	}

	if err := writePRMeta(c.repo(), branchName, meta); err != nil {
		return fmt.Errorf("recording PR metadata: %w", err)
	}

	fmt.Fprintf(c.out(), "Checked out PR #%d (%s) as branch '%s'.\n", prNumber, prType, branchName)
	return nil
}
