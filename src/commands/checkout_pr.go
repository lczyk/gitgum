package commands

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/lczyk/gitgum/src/internal"
)

// CheckoutPRCommand handles checking out pull requests from remotes
type CheckoutPRCommand struct{}

// PRRef represents a pull request reference
type PRRef struct {
	Number int
	Ref    string
	Type   string // "head" or "merge"
}

// Execute runs the checkout-pr command
func (c *CheckoutPRCommand) Execute(args []string) error {
	// Check if we're in a git repository
	if err := internal.CheckInGitRepo(); err != nil {
		return err
	}

	// Get list of remotes
	remotes, err := internal.GetRemotes()
	if err != nil {
		return fmt.Errorf("error getting remotes: %v", err)
	}

	if len(remotes) == 0 {
		fmt.Fprintln(os.Stderr, "No remotes found. Aborting checkout-pr.")
		return fmt.Errorf("no remotes")
	}

	// Select a remote
	remote, err := internal.FzfSelect("Select a remote to fetch PR from", remotes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No remote selected. Aborting checkout-pr.")
		return err
	}

	// Get PR refs from remote
	prRefs, err := getPRRefs(remote)
	if err != nil {
		return fmt.Errorf("error getting PR refs: %v", err)
	}

	if len(prRefs) == 0 {
		fmt.Fprintln(os.Stderr, "No pull requests found on remote. Aborting checkout-pr.")
		return fmt.Errorf("no pull requests found")
	}

	// Format PR refs for display
	prOptions := formatPROptions(prRefs)

	// Let user select a PR
	selected, err := internal.FzfSelect("Select a pull request to checkout", prOptions)
	if err != nil {
		fmt.Fprintln(os.Stderr, "No PR selected. Aborting checkout-pr.")
		return err
	}

	// Extract PR number from selection
	prNumber, prType, err := parsePRSelection(selected)
	if err != nil {
		return err
	}

	// Fetch and checkout the PR
	return checkoutPR(remote, prNumber, prType)
}

// getPRRefs fetches pull request references from a remote
func getPRRefs(remote string) ([]PRRef, error) {
	fmt.Println("Fetching pull request references from remote:", remote)
	stdout, _, err := internal.RunCommand("git", "ls-remote", remote)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote refs: %v", err)
	}

	// Parse PR refs from output
	// Example: d8debe2561cf2f995e5f11064e0da6201856ed6b	refs/pull/436/merge
	prRegex := regexp.MustCompile(`^[a-f0-9]+\s+refs/pull/(\d+)/(head|merge)$`)

	var prRefs []PRRef
	prMap := make(map[int]PRRef)

	for _, line := range internal.SplitLines(stdout) {
		matches := prRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			prNumber, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			prType := matches[2]

			// We prefer "head" over "merge", so only add if not already present
			// or if current is "merge" and new is "head"
			if existing, exists := prMap[prNumber]; !exists || (existing.Type == "merge" && prType == "head") {
				prMap[prNumber] = PRRef{
					Number: prNumber,
					Ref:    fmt.Sprintf("refs/pull/%d/%s", prNumber, prType),
					Type:   prType,
				}
			}
		}
	}

	// Convert map to slice and sort by PR number (descending)
	for _, pr := range prMap {
		prRefs = append(prRefs, pr)
	}
	sort.Slice(prRefs, func(i, j int) bool {
		return prRefs[i].Number > prRefs[j].Number
	})

	return prRefs, nil
}

// formatPROptions formats PR refs for display in fzf
func formatPROptions(prRefs []PRRef) []string {
	options := make([]string, len(prRefs))
	for i, pr := range prRefs {
		options[i] = fmt.Sprintf("PR #%d (%s)", pr.Number, pr.Type)
	}
	return options
}

// parsePRSelection extracts PR number and type from the user's selection
func parsePRSelection(selection string) (int, string, error) {
	// Parse "PR #123 (head)" format
	re := regexp.MustCompile(`^PR #(\d+) \((\w+)\)$`)
	matches := re.FindStringSubmatch(selection)
	if len(matches) != 3 {
		return 0, "", fmt.Errorf("invalid PR selection format: %s", selection)
	}

	prNumber, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid PR number: %s", matches[1])
	}

	return prNumber, matches[2], nil
}

// checkoutPR fetches and checks out a pull request
func checkoutPR(remote string, prNumber int, prType string) error {
	branchName := fmt.Sprintf("pr-%d", prNumber)
	prRef := fmt.Sprintf("refs/pull/%d/%s", prNumber, prType)

	// Check if branch already exists locally
	if internal.BranchExists(branchName) {
		// Ask user if they want to reset it
		confirmed, err := internal.FzfConfirm(
			fmt.Sprintf("Branch '%s' already exists. Reset it to the latest PR state?", branchName),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			// Just checkout the existing branch
			if err := internal.RunCommandWithOutput("git", "checkout", branchName); err != nil {
				return fmt.Errorf("failed to checkout existing branch '%s': %v", branchName, err)
			}
			fmt.Printf("Switched to existing branch '%s'.\n", branchName)
			return nil
		}

		// Fetch and reset
		fmt.Printf("Fetching PR #%d from %s...\n", prNumber, remote)
		if err := internal.RunCommandWithOutput("git", "fetch", remote, prRef); err != nil {
			return fmt.Errorf("failed to fetch PR: %v", err)
		}

		if err := internal.RunCommandWithOutput("git", "checkout", branchName); err != nil {
			return fmt.Errorf("failed to checkout branch '%s': %v", branchName, err)
		}

		if err := internal.RunCommandWithOutput("git", "reset", "--hard", "FETCH_HEAD"); err != nil {
			return fmt.Errorf("failed to reset branch: %v", err)
		}

		fmt.Printf("Reset branch '%s' to PR #%d (%s).\n", branchName, prNumber, prType)
		return nil
	}

	// Check for uncommitted changes
	dirty, err := internal.IsGitDirty(".")
	if err != nil {
		return fmt.Errorf("error checking git status: %v", err)
	}
	if dirty {
		fmt.Fprintln(os.Stderr, "You have local changes that would be overwritten. Please commit or stash them before checking out a PR.")
		return fmt.Errorf("local changes would be overwritten")
	}

	// Fetch and checkout as new branch
	fmt.Printf("Fetching PR #%d from %s...\n", prNumber, remote)
	if err := internal.RunCommandWithOutput("git", "fetch", remote, prRef); err != nil {
		return fmt.Errorf("failed to fetch PR: %v", err)
	}

	if err := internal.RunCommandWithOutput("git", "checkout", "-b", branchName, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("failed to create and checkout branch: %v", err)
	}

	fmt.Printf("Checked out PR #%d (%s) as branch '%s'.\n", prNumber, prType, branchName)
	return nil
}
