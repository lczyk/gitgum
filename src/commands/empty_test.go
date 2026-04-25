package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// EmptyCommand tests validate basic command structure.
// Full E2E testing requires mocking the fuzzyfinder picker (user input).

func TestEmptyCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "repository")
}

func TestEmptyCommand_NoUpstream(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err == nil, "should succeed without upstream, got error")
}
