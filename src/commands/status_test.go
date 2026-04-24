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
