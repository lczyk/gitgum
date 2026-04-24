package commands

import (
	"testing"
)

func TestParseRemotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name: "single remote with fetch and push",
			input: `origin	https://github.com/user/repo.git (fetch)
origin	https://github.com/user/repo.git (push)`,
			expected: []string{"origin https://github.com/user/repo.git"},
		},
		{
			name: "multiple remotes",
			input: `origin	https://github.com/user/repo.git (fetch)
origin	https://github.com/user/repo.git (push)
upstream	https://github.com/upstream/repo.git (fetch)
upstream	https://github.com/upstream/repo.git (push)`,
			expected: []string{
				"origin https://github.com/user/repo.git",
				"upstream https://github.com/upstream/repo.git",
			},
		},
		{
			name: "different fetch and push URLs",
			input: `origin	https://github.com/user/repo.git (fetch)
origin	git@github.com:user/repo.git (push)`,
			expected: []string{
				"origin https://github.com/user/repo.git",
				"origin git@github.com:user/repo.git",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRemotes(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("parseRemotes() got %d results, want %d", len(result), len(tt.expected))
				t.Logf("Got: %v", result)
				t.Logf("Want: %v", tt.expected)
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("parseRemotes() result[%d] = %q, want %q", i, result[i], expected)
				}
			}
		})
	}
}

func TestIsCommandAvailable(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "git should be available",
			command:  "git",
			expected: true,
		},
		{
			name:     "nonexistent command",
			command:  "this-command-definitely-does-not-exist-12345",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCommandAvailable(tt.command)
			if result != tt.expected {
				t.Errorf("isCommandAvailable(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}
