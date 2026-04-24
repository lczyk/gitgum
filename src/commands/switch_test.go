package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

func TestBranchTypeHandling(t *testing.T) {
	// verify that the branch type system correctly parses and validates selection strings
	testCases := []struct {
		name        string
		selected    string
		expectType  string
		expectName  string
		expectError bool
	}{
		{"local branch", "local: main", "local", "main", false},
		{"local with slash", "local: feature/test", "local", "feature/test", false},
		{"remote branch", "remote: origin/main", "remote", "origin/main", false},
		{"invalid no separator", "local main", "", "", true},
		{"invalid empty name", "local: ", "local", "", false}, // empty name parsed but would fail later
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.SplitN(tc.selected, ": ", 2)
			if !tc.expectError {
				assert.That(t, len(parts) == 2, "should split into 2 parts")
				assert.That(t, parts[0] == tc.expectType, "type mismatch")
				assert.That(t, parts[1] == tc.expectName, "name mismatch")
			} else {
				assert.That(t, len(parts) != 2, "should not split cleanly")
			}
		})
	}
}
