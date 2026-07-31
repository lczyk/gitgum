package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
)

func TestEmptyCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &EmptyCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestEmptyCommand_NoUpstream(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &EmptyCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	require.NoError(t, err, "should succeed without upstream")
}

// The push question is asked before the commit exists, so cancelling it has
// nothing to undo -- the old order committed first and then reported failure
// over work that had already succeeded.
func TestEmptyCommand_CancelLeavesHeadAlone(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))

	stub := &stubSelector{confirmErrs: []error{ui.ErrCancelled}}
	cmd := &EmptyCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}

	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled, "cancelling stops the command")
	after := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.Equal(t, after, before, "no commit should have been created")
}

// Declining is not cancelling: the commit is the work, so it still happens.
func TestEmptyCommand_DecliningPushStillCommits(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))

	stub := &stubSelector{confirmAnswers: []bool{false}}
	cmd := &EmptyCommand{cmdIO: cmdIO{UI: stub, Out: &strings.Builder{}, Repo: git.Repo{Dir: local}}}

	require.NoError(t, cmd.Execute(nil))
	after := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	assert.That(t, after != before, "the commit should still have been made")
}
