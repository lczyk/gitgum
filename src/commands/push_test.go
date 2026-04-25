package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestPushCommand_NotInGitRepo(t *testing.T) {
	temp_repo.ChdirTempDir(t)

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestPushCommand_NoRemotes(t *testing.T) {
	temp_repo.InitTempRepo(t)

	cmd := &PushCommand{}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no remotes")
	assert.ContainsString(t, err.Error(), "no remotes")
}

// when local matches remote but no tracking is configured, push sets the
// upstream without prompting (no UI interaction needed).
func TestPushCommand_AlreadyUpToDate_SetsUpstream(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	temp_repo.RunGit(t, dir, "push", "origin", branch)

	cmd := &PushCommand{}
	err := cmd.Execute([]string{"origin"})
	assert.NoError(t, err, "should succeed when already up to date")

	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "origin/"+branch)
}
