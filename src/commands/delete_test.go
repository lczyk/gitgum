package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestDeleteCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &DeleteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestDeleteCommand_NoBranches(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewEmptyRepo(t)

	cmd := &DeleteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when no branches exist")
	assert.ContainsString(t, err.Error(), "no local branches")
}

// End-to-end test driven by a stub Selector: picks a non-current feature
// branch and deletes it without any further prompts. Demonstrates that
// commands can be exercised end-to-end without a TTY.
func TestDeleteCommand_DeletesPickedBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{"local: feature"}}
	cmd := &DeleteCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	branches := temp_repo.RunGit(t, dir, "branch", "--list", "feature")
	assert.Equal(t, strings.TrimSpace(branches), "")
	assert.ContainsString(t, buf.String(), "Deleted local branch 'feature'.")
	assert.Equal(t, len(stub.confirmCalls), 0)
}

// Deleting a remote branch the remote does not have is a real refusal from
// git, and the wrapped error has to surface rather than reading as success.
func TestDeleteCommand_RemoteDeleteFailureSurfaces(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)

	stub := &stubSelector{confirmAnswers: []bool{true}}
	cmd := &DeleteCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	// the remote has no branches at all, so deleting one must fail
	err := cmd.deleteRemoteOnly("origin", "never-existed")
	assert.Error(t, err, assert.AnyError, "git should refuse to delete a missing remote branch")
	assert.ContainsString(t, err.Error(), "deleting remote branch")
}
