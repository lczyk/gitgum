package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestPushCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestPushCommand_NoRemotes(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when no remotes")
	assert.ContainsString(t, err.Error(), "no remotes")
}
