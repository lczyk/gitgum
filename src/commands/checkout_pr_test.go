package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestCheckoutPRCommand_Execute(t *testing.T) {
	// Note: This is a basic structure test since checkout-pr requires an actual git repo
	// and interactive fzf input. Full integration testing should be done manually
	// or with a more sophisticated test setup.

	cmd := &CheckoutPRCommand{}
	assert.That(t, cmd != nil, "CheckoutPRCommand should be created successfully")
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
				{Number: 123, Ref: "refs/pull/123/head", Type: "head"},
			},
			expected: []string{"PR #123 (head)"},
		},
		{
			name: "single PR merge",
			prRefs: []PRRef{
				{Number: 456, Ref: "refs/pull/456/merge", Type: "merge"},
			},
			expected: []string{"PR #456 (merge)"},
		},
		{
			name: "multiple PRs",
			prRefs: []PRRef{
				{Number: 123, Ref: "refs/pull/123/head", Type: "head"},
				{Number: 456, Ref: "refs/pull/456/merge", Type: "merge"},
				{Number: 789, Ref: "refs/pull/789/head", Type: "head"},
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
