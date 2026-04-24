package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestStatusCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &StatusCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestStatusCommand_InGitRepo(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &StatusCommand{}
	err := cmd.Execute(nil)

	assert.NoError(t, err, "should succeed in git repo")
}

func TestParseRemotes(t *testing.T) {
	cases := map[string]struct {
		input    string
		expected []string
	}{
		"empty input":                    {input: "", expected: []string{}},
		"incomplete line with only name": {input: "origin", expected: []string{}},
		"whitespace-only line":           {input: "   \t   ", expected: []string{}},
		"mixed valid and incomplete lines": {
			input: "origin\thttps://github.com/user/repo.git (fetch)\nincomplete-line\nupstream\thttps://github.com/upstream/repo.git (fetch)",
			expected: []string{
				"origin https://github.com/user/repo.git",
				"upstream https://github.com/upstream/repo.git",
			},
		},
		"deduplicates identical entries": {
			input:    "origin\thttps://example.com (fetch)\norigin\thttps://example.com (push)\norigin\thttps://example.com (fetch)",
			expected: []string{"origin https://example.com"},
		},
		"single remote with fetch and push": {
			input:    "origin\thttps://github.com/user/repo.git (fetch)\norigin\thttps://github.com/user/repo.git (push)",
			expected: []string{"origin https://github.com/user/repo.git"},
		},
		"multiple remotes": {
			input: "origin\thttps://github.com/user/repo.git (fetch)\norigin\thttps://github.com/user/repo.git (push)\nupstream\thttps://github.com/upstream/repo.git (fetch)\nupstream\thttps://github.com/upstream/repo.git (push)",
			expected: []string{
				"origin https://github.com/user/repo.git",
				"upstream https://github.com/upstream/repo.git",
			},
		},
		"different fetch and push URLs": {
			input: "origin\thttps://github.com/user/repo.git (fetch)\norigin\tgit@github.com:user/repo.git (push)",
			expected: []string{
				"origin https://github.com/user/repo.git",
				"origin git@github.com:user/repo.git",
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			result := parseRemotes(tt.input)
			assert.EqualArrays(t, result, tt.expected)
		})
	}
}
