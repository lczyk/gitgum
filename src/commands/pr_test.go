package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/pr"
)

func TestSwitchUnselectable(t *testing.T) {
	t.Parallel()
	// current branch (HEAD row): selectable now (picking it pulls).
	assert.That(t, !switchUnselectable("local: main"+currentBranchMarker), "current branch should be selectable")
	// branch checked out in another worktree: still blocked.
	assert.That(t, switchUnselectable("local: feat"+checkedOutSuffix("/wt/feat-2")), "other-worktree checkout stays blocked")
	// detached HEAD row: still blocked.
	assert.That(t, switchUnselectable(detachedEntry("cf10b5f")), "detached HEAD stays blocked")
	// a plain branch: selectable.
	assert.That(t, !switchUnselectable("local: other"), "plain branch selectable")
}

func TestFormatPROptions(t *testing.T) {
	cases := map[string]struct {
		prRefs   []pr.Ref
		expected []string
	}{
		"single PR head": {
			prRefs:   []pr.Ref{{Number: 123, Type: "head"}},
			expected: []string{"PR #123 (head)"},
		},
		"single PR merge": {
			prRefs:   []pr.Ref{{Number: 456, Type: "merge"}},
			expected: []string{"PR #456 (merge)"},
		},
		"multiple PRs": {
			prRefs: []pr.Ref{
				{Number: 123, Type: "head"},
				{Number: 456, Type: "merge"},
				{Number: 789, Type: "head"},
			},
			expected: []string{"PR #123 (head)", "PR #456 (merge)", "PR #789 (head)"},
		},
		"empty list": {prRefs: []pr.Ref{}, expected: []string{}},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualArrays(t, formatPROptions(tt.prRefs), tt.expected)
		})
	}
}

func TestParsePRSelection(t *testing.T) {
	cases := map[string]struct {
		selection      string
		expectedNum    int
		expectedType   string
		expectedError  bool
		expectedErrMsg string
	}{
		"valid head selection":            {selection: "PR #123 (head)", expectedNum: 123, expectedType: "head"},
		"valid merge selection":           {selection: "PR #456 (merge)", expectedNum: 456, expectedType: "merge"},
		"large PR number":                 {selection: "PR #9999 (head)", expectedNum: 9999, expectedType: "head"},
		"invalid format - missing type":   {selection: "PR #123", expectedError: true, expectedErrMsg: "invalid PR selection format"},
		"invalid format - wrong format":   {selection: "#123 (head)", expectedError: true, expectedErrMsg: "invalid PR selection format"},
		"invalid format - non-numeric PR": {selection: "PR #abc (head)", expectedError: true, expectedErrMsg: "invalid PR selection format"},
		"invalid PR type":                 {selection: "PR #123 (foo)", expectedError: true, expectedErrMsg: "invalid PR selection format"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			num, prType, err := parsePRSelection(tt.selection)

			if tt.expectedError {
				assert.Error(t, err, tt.expectedErrMsg, "expected error for %s", tt.selection)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedNum, num)
				assert.Equal(t, tt.expectedType, prType)
			}
		})
	}
}
