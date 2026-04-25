package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestDeleteCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestDeleteCommand_NoBranches(t *testing.T) {
	temp_repo.InitEmptyTempRepo(t)

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no branches exist")
	assert.ContainsString(t, err.Error(), "no branches")
}
