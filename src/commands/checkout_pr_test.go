package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestParsePRRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []PRRef
	}{
		{
			name:     "empty output",
			input:    "",
			expected: []PRRef{},
		},
		{
			name:  "single head ref",
			input: "abc123def456\trefs/pull/42/head",
			expected: []PRRef{
				{Number: 42, Type: "head"},
			},
		},
		{
			name:  "single merge ref",
			input: "abc123def456\trefs/pull/10/merge",
			expected: []PRRef{
				{Number: 10, Type: "merge"},
			},
		},
		{
			name: "head wins over merge for same PR",
			input: "aaa\trefs/pull/5/merge\n" +
				"bbb\trefs/pull/5/head",
			expected: []PRRef{
				{Number: 5, Type: "head"},
			},
		},
		{
			name: "head already present ignores later merge",
			input: "aaa\trefs/pull/5/head\n" +
				"bbb\trefs/pull/5/merge",
			expected: []PRRef{
				{Number: 5, Type: "head"},
			},
		},
		{
			name: "multiple PRs sorted descending",
			input: "aaa\trefs/pull/1/head\n" +
				"bbb\trefs/pull/99/merge\n" +
				"ccc\trefs/pull/50/head",
			expected: []PRRef{
				{Number: 99, Type: "merge"},
				{Number: 50, Type: "head"},
				{Number: 1, Type: "head"},
			},
		},
		{
			name: "non-PR refs ignored",
			input: "aaa\trefs/heads/main\n" +
				"bbb\trefs/tags/v1.0\n" +
				"ccc\trefs/pull/7/head",
			expected: []PRRef{
				{Number: 7, Type: "head"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePRRefs(tt.input)
			assert.That(t, len(result) == len(tt.expected),
				"parsePRRefs returned %d refs, want %d", len(result), len(tt.expected))
			for i, ref := range result {
				assert.That(t, ref.Number == tt.expected[i].Number,
					"ref[%d].Number = %d, want %d", i, ref.Number, tt.expected[i].Number)
				assert.That(t, ref.Type == tt.expected[i].Type,
					"ref[%d].Type = %q, want %q", i, ref.Type, tt.expected[i].Type)
			}
		})
	}
}

func TestFormatPROptions(t *testing.T) {
	tests := []struct {
		name     string
		prRefs   []PRRef
		expected []string
	}{
		{
			name: "single PR head",
			prRefs: []PRRef{
				{Number: 123, Type: "head"},
			},
			expected: []string{"PR #123 (head)"},
		},
		{
			name: "single PR merge",
			prRefs: []PRRef{
				{Number: 456, Type: "merge"},
			},
			expected: []string{"PR #456 (merge)"},
		},
		{
			name: "multiple PRs",
			prRefs: []PRRef{
				{Number: 123, Type: "head"},
				{Number: 456, Type: "merge"},
				{Number: 789, Type: "head"},
			},
			expected: []string{"PR #123 (head)", "PR #456 (merge)", "PR #789 (head)"},
		},
		{
			name:     "empty list",
			prRefs:   []PRRef{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPROptions(tt.prRefs)
			assert.That(t, len(result) == len(tt.expected), "formatPROptions should return correct number of options")
			for i, option := range result {
				assert.That(t, option == tt.expected[i], "formatPROptions should format option correctly")
			}
		})
	}
}

func TestParsePRSelection(t *testing.T) {
	tests := []struct {
		name          string
		selection     string
		expectedNum   int
		expectedType  string
		expectedError bool
	}{
		{
			name:          "valid head selection",
			selection:     "PR #123 (head)",
			expectedNum:   123,
			expectedType:  "head",
			expectedError: false,
		},
		{
			name:          "valid merge selection",
			selection:     "PR #456 (merge)",
			expectedNum:   456,
			expectedType:  "merge",
			expectedError: false,
		},
		{
			name:          "large PR number",
			selection:     "PR #9999 (head)",
			expectedNum:   9999,
			expectedType:  "head",
			expectedError: false,
		},
		{
			name:          "invalid format - missing type",
			selection:     "PR #123",
			expectedNum:   0,
			expectedType:  "",
			expectedError: true,
		},
		{
			name:          "invalid format - wrong format",
			selection:     "#123 (head)",
			expectedNum:   0,
			expectedType:  "",
			expectedError: true,
		},
		{
			name:          "invalid format - non-numeric PR",
			selection:     "PR #abc (head)",
			expectedNum:   0,
			expectedType:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, prType, err := parsePRSelection(tt.selection)
			
			if tt.expectedError {
				assert.That(t, err != nil, "parsePRSelection should return error for invalid input")
			} else {
				assert.That(t, err == nil, "parsePRSelection should not return error for valid input")
				assert.That(t, num == tt.expectedNum, "parsePRSelection should return correct PR number")
				assert.That(t, prType == tt.expectedType, "parsePRSelection should return correct PR type")
			}
		})
	}
}
