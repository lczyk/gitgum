package commands

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/lczyk/gitgum/internal/pr"
)

// prSelectionRegex recovers a PR from the picker label formatPROptions renders.
var prSelectionRegex = regexp.MustCompile(`^PR #(\d+) \((head|merge)\)$`)

// formatPROptions renders PR refs as picker labels ("PR #N (head)").
func formatPROptions(prRefs []pr.Ref) []string {
	options := make([]string, len(prRefs))
	for i, ref := range prRefs {
		options[i] = fmt.Sprintf("PR #%d (%s)", ref.Number, ref.Type)
	}
	return options
}

// parsePRSelection recovers the PR number and type from a picker label.
func parsePRSelection(selection string) (int, string, error) {
	matches := prSelectionRegex.FindStringSubmatch(selection)
	if len(matches) != 3 {
		return 0, "", fmt.Errorf("invalid PR selection format: %s", selection)
	}
	prNumber, _ := strconv.Atoi(matches[1]) // regex guarantees \d+
	prType := matches[2]
	return prNumber, prType, nil
}
