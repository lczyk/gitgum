package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestEmptyCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestEmptyCommand_NoUpstream(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.NoError(t, err, "should succeed without upstream")
}
