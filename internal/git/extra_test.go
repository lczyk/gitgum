package git_test

import (
	"slices"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestIsBranchAheadOfRemote(t *testing.T) {
	t.Parallel()

	t.Run("ahead", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)
		temp_repo.CreateCommit(t, local, "ahead.txt", "x", "chore: ahead commit")

		ahead, err := git.Repo{Dir: local}.IsBranchAheadOfRemote("main", "origin/main")
		require.NoError(t, err)
		assert.That(t, ahead, "local should be ahead of remote after new commit")
	})

	t.Run("up to date", func(t *testing.T) {
		t.Parallel()
		local, _ := temp_repo.NewRepoWithRemote(t)

		ahead, err := git.Repo{Dir: local}.IsBranchAheadOfRemote("main", "origin/main")
		require.NoError(t, err)
		assert.That(t, !ahead, "freshly cloned local should not be ahead")
	})
}

func TestGetRemoteBranches(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	branches, err := git.Repo{Dir: local}.GetRemoteBranches("origin")
	require.NoError(t, err)
	assert.That(t, slices.Contains(branches, "main"), "main present in origin remote branches")
}

func TestGetRemoteBranches_UnknownRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	branches, err := git.Repo{Dir: dir}.GetRemoteBranches("nope")
	require.NoError(t, err)
	assert.Equal(t, len(branches), 0)
}

func TestBranchMerged(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	// a branch at HEAD is trivially merged
	temp_repo.RunGit(t, dir, "branch", "at-head")
	assert.That(t, r.BranchMerged("at-head"), "a branch at HEAD is merged")

	// one carrying its own commit is not
	temp_repo.RunGit(t, dir, "checkout", "-q", "-b", "diverged")
	temp_repo.CreateCommit(t, dir, "only-here.txt", "x", "chore: only here")
	temp_repo.RunGit(t, dir, "checkout", "-q", "main")
	assert.That(t, !r.BranchMerged("diverged"), "a branch with unmerged commits is not merged")

	assert.That(t, !r.BranchMerged("no-such-branch"), "an unknown branch is not merged")
}
