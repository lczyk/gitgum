package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestEmptyCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "repository")
}

func TestEmptyCommand_NoUpstream(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error without upstream")
	assert.ContainsString(t, err.Error(), "upstream")
}
