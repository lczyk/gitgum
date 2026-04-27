package commands

import (
	"os/exec"
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

// New remote branch flow: no upstream, no matching remote branch, user
// confirms creation. Stub answers the create-remote-branch prompt with yes.
func TestPushCommand_CreatesRemoteBranch(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)

	var buf strings.Builder
	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &PushCommand{cmdIO: cmdIO{Out: &buf, UI: stub}}

	err := cmd.Execute([]string{"origin"})
	assert.NoError(t, err)

	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "origin/"+branch)
	assert.ContainsString(t, buf.String(), "Created and set tracking reference")
	assert.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No remote branch")
}

// User declines the create-remote-branch confirmation: command exits cleanly
// without pushing or setting upstream.
func TestPushCommand_DeclinesCreateRemoteBranch(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)

	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &PushCommand{cmdIO: cmdIO{UI: stub}}

	err := cmd.Execute([]string{"origin"})
	assert.NoError(t, err)

	upstreamCmd := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{u}")
	upstreamCmd.Dir = dir
	assert.Error(t, upstreamCmd.Run(), assert.AnyError, "upstream must not be set when user declines")
}

// when local matches remote but no tracking is configured, push sets the
// upstream without prompting (no UI interaction needed).
func TestPushCommand_AlreadyUpToDate_SetsUpstream(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	branch := currentBranchIn(t, dir)
	temp_repo.RunGit(t, dir, "push", "origin", branch)

	cmd := &PushCommand{}
	err := cmd.Execute([]string{"origin"})
	assert.NoError(t, err, "should succeed when already up to date")

	upstream := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--abbrev-ref", branch+"@{u}"))
	assert.Equal(t, upstream, "origin/"+branch)
}
