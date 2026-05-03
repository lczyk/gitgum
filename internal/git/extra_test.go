package git_test

import (
	"slices"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestIsBranchAheadOfRemote(t *testing.T) {
	t.Parallel()

	t.Run("ahead", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "ahead.txt", "x", "ahead commit")

		ahead, err := git.Repo{Dir: local}.IsBranchAheadOfRemote("main", "origin/main")
		assert.NoError(t, err)
		assert.That(t, ahead, "local should be ahead of remote after new commit")
	})

	t.Run("up to date", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)

		ahead, err := git.Repo{Dir: local}.IsBranchAheadOfRemote("main", "origin/main")
		assert.NoError(t, err)
		assert.That(t, !ahead, "freshly cloned local should not be ahead")
	})
}

func TestGetRemoteBranches(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	branches, err := git.Repo{Dir: local}.GetRemoteBranches("origin")
	assert.NoError(t, err)
	assert.That(t, slices.Contains(branches, "main"), "main present in origin remote branches")
}

func TestGetRemoteBranches_UnknownRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	branches, err := git.Repo{Dir: dir}.GetRemoteBranches("nope")
	assert.NoError(t, err)
	assert.Equal(t, len(branches), 0)
}

// Free-function GetFileStatus operates on process cwd, so this test cannot
// run in parallel (ChdirTempDir + InitTempRepo serialise on cwd).
func TestGetFileStatus_FreeFn(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.WriteFile(t, dir, "untracked.txt", "x")

	status, err := git.GetFileStatus("untracked.txt")
	assert.NoError(t, err)
	assert.Equal(t, status, git.FileUntracked)
}
