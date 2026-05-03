package commands

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/lczyk/gitgum/internal/strutil"
)

var (
	prRegex          = regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)
	prSelectionRegex = regexp.MustCompile(`^PR #(\d+) \((head|merge)\)$`)
)

type CheckoutPRCommand struct {
	cmdIO
}

type PRRef struct {
	Number int
	Type   string // "head" or "merge"
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

	remote, err := c.sel().Select("Select a remote to fetch PR from", remotes)
	if err != nil {
		fmt.Fprintln(c.err(), "No remote selected. Aborting checkout-pr.")
		return err
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

// parsePRRefs extracts PR refs from `git ls-remote` output.
// when both head and merge exist for a PR, head wins.
func parsePRRefs(lsRemoteOutput string) []PRRef {
	prMap := make(map[int]PRRef)

	for _, line := range strutil.SplitLines(lsRemoteOutput) {
		matches := prRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		prNumber, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
		prType := matches[2]

		existing, found := prMap[prNumber]
		if !found || (existing.Type == "merge" && prType == "head") {
			prMap[prNumber] = PRRef{Number: prNumber, Type: prType}
		}
	}

	prRefs := make([]PRRef, 0, len(prMap))
	for _, pr := range prMap {
		prRefs = append(prRefs, pr)
	}
	sort.Slice(prRefs, func(i, j int) bool {
		return prRefs[i].Number > prRefs[j].Number
	})

	return prRefs
}

func formatPROptions(prRefs []PRRef) []string {
	options := make([]string, len(prRefs))
	for i, pr := range prRefs {
		options[i] = fmt.Sprintf("PR #%d (%s)", pr.Number, pr.Type)
	}
	return options
}

func parsePRSelection(selection string) (int, string, error) {
	matches := prSelectionRegex.FindStringSubmatch(selection)
	if len(matches) != 3 {
		return 0, "", fmt.Errorf("invalid PR selection format: %s", selection)
	}

	prNumber, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
	prType := matches[2]
	return prNumber, prType, nil
}

func (c *CheckoutPRCommand) checkoutPR(remote string, prNumber int, prType string) error {
	branchName := fmt.Sprintf("pr-%d", prNumber)
	prRef := fmt.Sprintf("refs/pull/%d/%s", prNumber, prType)

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
			fmt.Fprintf(c.out(), "Switched to existing branch '%s'.\n", branchName)
			return nil
		}

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

	fmt.Fprintf(c.out(), "Checked out PR #%d (%s) as branch '%s'.\n", prNumber, prType, branchName)
	return nil
}
