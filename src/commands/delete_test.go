package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestDeleteCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestDeleteCommand_NoBranches(t *testing.T) {
	// Initialize a repo without any branches (just init, no commits)
	dir := temp_repo.ChdirTempDir(t)

	temp_repo.RunGit(t, dir, "init")
	temp_repo.RunGit(t, dir, "config", "user.name", "Test User")
	temp_repo.RunGit(t, dir, "config", "user.email", "test@example.com")

	cmd := &DeleteCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error when no branches exist")
	assert.ContainsString(t, err.Error(), "no branches")
}
