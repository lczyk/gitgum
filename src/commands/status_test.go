package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
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
			assert.EqualArrays(t, result, tt.expected)
		})
	}
}
