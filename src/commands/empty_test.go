package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/internal/temp_repo"
)

func TestEmptyCommand_NoUpstream(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should error without upstream")
	assert.ContainsString(t, err.Error(), "upstream")
}

func TestEmptyCommand_AheadOfRemote(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)

	// set up bare remote and push to establish tracking
	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)
	branch := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	temp_repo.RunGit(t, dir, "push", "-u", "origin", branch)

	// make local branch ahead of remote
	temp_repo.CreateCommit(t, dir, "extra.txt", "content", "extra commit")

	cmd := &EmptyCommand{}
	err := cmd.Execute(nil)

	assert.That(t, err != nil, "should refuse when ahead of remote")
	assert.ContainsString(t, err.Error(), "ahead")
}
