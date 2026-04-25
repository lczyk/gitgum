package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestParsePRRefs(t *testing.T) {
	cases := map[string]struct {
		input    string
		expected []PRRef
	}{
		"empty output": {input: "", expected: []PRRef{}},
		"single head ref": {
			input:    "abc123def456\trefs/pull/42/head",
			expected: []PRRef{{Number: 42, Type: "head"}},
		},
		"single merge ref": {
			input:    "abc123def456\trefs/pull/10/merge",
			expected: []PRRef{{Number: 10, Type: "merge"}},
		},
		"head wins over merge for same PR": {
			input:    "aaa\trefs/pull/5/merge\nbbb\trefs/pull/5/head",
			expected: []PRRef{{Number: 5, Type: "head"}},
		},
		"head already present ignores later merge": {
			input:    "aaa\trefs/pull/5/head\nbbb\trefs/pull/5/merge",
			expected: []PRRef{{Number: 5, Type: "head"}},
		},
		"multiple PRs sorted descending": {
			input: "aaa\trefs/pull/1/head\nbbb\trefs/pull/99/merge\nccc\trefs/pull/50/head",
			expected: []PRRef{
				{Number: 99, Type: "merge"},
				{Number: 50, Type: "head"},
				{Number: 1, Type: "head"},
			},
		},
		"non-PR refs ignored": {
			input:    "aaa\trefs/heads/main\nbbb\trefs/tags/v1.0\nccc\trefs/pull/7/head",
			expected: []PRRef{{Number: 7, Type: "head"}},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualArrays(t, parsePRRefs(tt.input), tt.expected)
		})
	}
}

func TestFormatPROptions(t *testing.T) {
	cases := map[string]struct {
		prRefs   []PRRef
		expected []string
	}{
		"single PR head": {
			prRefs:   []PRRef{{Number: 123, Type: "head"}},
			expected: []string{"PR #123 (head)"},
		},
		"single PR merge": {
			prRefs:   []PRRef{{Number: 456, Type: "merge"}},
			expected: []string{"PR #456 (merge)"},
		},
		"multiple PRs": {
			prRefs: []PRRef{
				{Number: 123, Type: "head"},
				{Number: 456, Type: "merge"},
				{Number: 789, Type: "head"},
			},
			expected: []string{"PR #123 (head)", "PR #456 (merge)", "PR #789 (head)"},
		},
		"empty list": {prRefs: []PRRef{}, expected: []string{}},
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedNum, num)
				assert.Equal(t, tt.expectedType, prType)
			}
		})
	}
}
