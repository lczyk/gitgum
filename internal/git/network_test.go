package git_test

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestFetch_BringsRemoteRefs(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)

	remoteHEAD := strings.TrimSpace(temp_repo.RunGit(t, remote, "rev-parse", "HEAD"))

	assert.NoError(t, git.Repo{Dir: local}.Fetch("origin", "main"))

	got := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "origin/main"))
	assert.Equal(t, got, remoteHEAD)
}

func TestFetch_UnknownRefspec(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	err := git.Repo{Dir: local}.Fetch("origin", "refs/heads/nonexistent")
	assert.Error(t, err, assert.AnyError, "missing refspec should error")
}

func TestPush_ToTrackingRemote(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	temp_repo.CreateCommit(t, local, "added.txt", "x", "chore: add added.txt")
	localHEAD := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))

	assert.NoError(t, git.Repo{Dir: local}.Push())

	remoteHEAD := strings.TrimSpace(temp_repo.RunGit(t, remote, "rev-parse", "main"))
	assert.Equal(t, remoteHEAD, localHEAD)
}

func TestLsRemote_ListsHeads(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	out, err := git.Repo{Dir: local}.LsRemote("origin")
	assert.NoError(t, err)
	assert.ContainsString(t, out, "refs/heads/main")
}

func TestLsRemote_BadURL(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)

	_, err := git.Repo{Dir: local}.LsRemote("/nonexistent/path/does/not/exist")
	assert.Error(t, err, assert.AnyError, "bad remote URL should error")
}
