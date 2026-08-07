package git_test

import (
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

// RemoteURLs answers what `git remote -v` shows, without the columns: a
// remote whose push url matches its fetch url is one entry, one whose push
// url differs is two.
func TestRemoteURLs(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")
	temp_repo.RunGit(t, dir, "remote", "add", "upstream", "https://example.com/up.git")

	got, err := git.Repo{Dir: dir}.RemoteURLs()
	require.NoError(t, err, "listing remote urls")
	assert.EqualArrays(t, got, []string{
		"origin https://example.com/repo.git",
		"upstream https://example.com/up.git",
	})
}

func TestRemoteURLs_SeparatePushURL(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")
	temp_repo.RunGit(t, dir, "remote", "set-url", "--push", "origin", "git@example.com:repo.git")

	got, err := git.Repo{Dir: dir}.RemoteURLs()
	require.NoError(t, err, "listing remote urls")
	assert.EqualArrays(t, got, []string{
		"origin git@example.com:repo.git",
		"origin https://example.com/repo.git",
	})
}

// A remote name may contain '.', which the config key then contains twice.
func TestRemoteURLs_DottedName(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "team.fork", "https://example.com/fork.git")

	got, err := git.Repo{Dir: dir}.RemoteURLs()
	require.NoError(t, err, "listing remote urls")
	assert.EqualArrays(t, got, []string{"team.fork https://example.com/fork.git"})
}

// A repo with no remotes has nothing to list, which is not a failure to list.
func TestRemoteURLs_NoRemotes(t *testing.T) {
	t.Parallel()
	got, err := git.Repo{Dir: temp_repo.NewRepo(t)}.RemoteURLs()
	require.NoError(t, err, "no remotes is not an error")
	assert.Equal(t, len(got), 0)
}
