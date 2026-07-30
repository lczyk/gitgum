package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lczyk/gitgum/internal/pr"
)

// CheckoutPRCommand checks a pull request out as a local branch. Naming one
// ("canonical/42") goes straight to it; with no argument it is the interactive
// dispatcher -- pick a remote, pick a PR. Either way the remote is listed
// first, because a PR number only identifies a PR alongside the remote it
// belongs to: 42 on a fork and 42 upstream are different pull requests.
type CheckoutPRCommand struct {
	cmdIO
	Args struct {
		PR string `positional-arg-name:"[REMOTE/]NUMBER"`
	} `positional-args:"yes"`
}

func (c *CheckoutPRCommand) Execute(args []string) error {
	if err := c.repo().CheckInRepo(); err != nil {
		return err
	}

	var wantRemote string
	var wantNumber int
	if token := strings.TrimSpace(c.Args.PR); token != "" {
		var ok bool
		if wantRemote, wantNumber, ok = pr.ParseToken(token); !ok {
			return fmt.Errorf("%q is not a pull request "+
				"(expected NUMBER, REMOTE/NUMBER, or pr/REMOTE/NUMBER)", token)
		}
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
	switch {
	case wantRemote != "":
		if !slices.Contains(remotes, wantRemote) {
			return fmt.Errorf("no remote named %q (have: %s)", wantRemote, strings.Join(remotes, ", "))
		}
		remote = wantRemote
	case len(remotes) == 1:
		remote = remotes[0] // only one remote: no point asking.
	default:
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

	// A named PR still goes through the listing: it is what says whether the PR
	// exists and which of head/merge the remote advertises for it.
	if wantNumber != 0 {
		for _, ref := range prRefs {
			if ref.Number == wantNumber {
				return c.checkoutPR(remote, ref.Number, ref.Type)
			}
		}
		return fmt.Errorf("remote %q has no pull request #%d", remote, wantNumber)
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

func (c *CheckoutPRCommand) getPRRefs(remote string) ([]pr.Ref, error) {
	fmt.Fprintln(c.out(), "Fetching pull request references from remote:", remote)
	stdout, err := c.repo().LsRemote(remote)
	if err != nil {
		return nil, fmt.Errorf("listing remote refs: %w", err)
	}

	return pr.ParseRefs(pr.ForgeOf(c.repo(), remote), stdout), nil
}

func (c *CheckoutPRCommand) checkoutPR(remote string, prNumber int, prType string) error {
	meta := pr.Meta{Remote: remote, Number: prNumber, Type: prType}
	branchName := pr.BranchName(remote, prNumber)
	prRef := meta.FetchRef(pr.ForgeOf(c.repo(), remote))

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
			if err := pr.WriteMeta(c.repo(), branchName, meta); err != nil {
				return fmt.Errorf("recording PR metadata: %w", err)
			}
			fmt.Fprintf(c.out(), "Switched to existing branch '%s'.\n", branchName)
			return nil
		}

		// The reset below is destructive, so deal with the working tree first.
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

		if err := pr.WriteMeta(c.repo(), branchName, meta); err != nil {
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

	if err := pr.WriteMeta(c.repo(), branchName, meta); err != nil {
		return fmt.Errorf("recording PR metadata: %w", err)
	}

	fmt.Fprintf(c.out(), "Checked out PR #%d (%s) as branch '%s'.\n", prNumber, prType, branchName)
	return nil
}
