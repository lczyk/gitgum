package commands

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/lczyk/gitgum/src/internal"
)

var (
	prRegex        = regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)
	prSelectionRex = regexp.MustCompile(`^PR #(\d+) \((\w+)\)$`)
)

type CheckoutPRCommand struct{}

type PRRef struct {
	Number int
	Type   string // "head" or "merge"
}

func (c *CheckoutPRCommand) Execute(args []string) error {
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(os.Stderr, "No remotes found. Aborting checkout-pr.")
		return fmt.Errorf("no remotes")
	}

	remote, err := internal.FzfSelect("Select a remote to fetch PR from", remotes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No remote selected. Aborting checkout-pr.")
		return err
	}

	prRefs, err := getPRRefs(remote)
	if err != nil {
		return fmt.Errorf("getting PR refs: %w", err)
	}

	if len(prRefs) == 0 {
		fmt.Fprintln(os.Stderr, "No pull requests found on remote. Aborting checkout-pr.")
		return fmt.Errorf("no pull requests found")
	}

	prOptions := formatPROptions(prRefs)

	selected, err := internal.FzfSelect("Select a pull request to checkout", prOptions)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No PR selected. Aborting checkout-pr.")
		return err
	}

	prNumber, prType, err := parsePRSelection(selected)
	if err != nil {
		return err
	}

	return checkoutPR(remote, prNumber, prType)
}

func getPRRefs(remote string) ([]PRRef, error) {
	fmt.Println("Fetching pull request references from remote:", remote)
	stdout, _, err := internal.RunCommand("git", "ls-remote", remote)
	if err != nil {
		return nil, fmt.Errorf("listing remote refs: %w", err)
	}

	return parsePRRefs(stdout), nil
}

// parsePRRefs extracts PR refs from `git ls-remote` output.
// when both head and merge exist for a PR, head wins.
func parsePRRefs(lsRemoteOutput string) []PRRef {
	prMap := make(map[int]PRRef)

	for _, line := range internal.SplitLines(lsRemoteOutput) {
		matches := prRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		prNumber, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		prType := matches[2]

		// prefer head over merge
		if existing, exists := prMap[prNumber]; !exists || (existing.Type == "merge" && prType == "head") {
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
	matches := prSelectionRex.FindStringSubmatch(selection)
	if len(matches) != 3 {
		return 0, "", fmt.Errorf("invalid PR selection format: %s", selection)
	}

	prNumber, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid PR number: %s", matches[1])
	}

	prType := matches[2]
	if prType != "head" && prType != "merge" {
		return 0, "", fmt.Errorf("invalid PR type: %s", prType)
	}

	return prNumber, prType, nil
}

func checkoutPR(remote string, prNumber int, prType string) error {
	branchName := fmt.Sprintf("pr-%d", prNumber)
	prRef := fmt.Sprintf("refs/pull/%d/%s", prNumber, prType)

	if internal.BranchExists(branchName) {
		confirmed, err := internal.FzfConfirm(
			fmt.Sprintf("Branch '%s' already exists. Reset it to the latest PR state?", branchName),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			if err := internal.RunCommandWithOutput("git", "checkout", branchName); err != nil {
				return fmt.Errorf("checking out existing branch '%s': %w", branchName, err)
			}
			fmt.Printf("Switched to existing branch '%s'.\n", branchName)
			return nil
		}

		fmt.Printf("Fetching PR #%d from %s...\n", prNumber, remote)
		if err := internal.RunCommandWithOutput("git", "fetch", remote, prRef); err != nil {
			return fmt.Errorf("fetching PR: %w", err)
		}

		if err := internal.RunCommandWithOutput("git", "checkout", branchName); err != nil {
			return fmt.Errorf("checking out branch '%s': %w", branchName, err)
		}

		if err := internal.RunCommandWithOutput("git", "reset", "--hard", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("resetting branch: %w", err)
		}

		fmt.Printf("Reset branch '%s' to PR #%d (%s).\n", branchName, prNumber, prType)
		return nil
	}

	dirty, err := internal.IsGitDirty(".")
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "You have local changes that would be overwritten. Please commit or stash them before checking out a PR.")
		return fmt.Errorf("local changes would be overwritten")
	}

	fmt.Printf("Fetching PR #%d from %s...\n", prNumber, remote)
	if err := internal.RunCommandWithOutput("git", "fetch", remote, prRef); err != nil {
		return fmt.Errorf("fetching PR: %w", err)
	}

	if err := internal.RunCommandWithOutput("git", "checkout", "-b", branchName, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("creating and checking out branch: %w", err)
	}

	fmt.Printf("Checked out PR #%d (%s) as branch '%s'.\n", prNumber, prType, branchName)
	return nil
}
