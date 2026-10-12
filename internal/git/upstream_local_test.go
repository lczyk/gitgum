package git_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// A branch tracking another local branch has the remote "." and an upstream
// short name with no remote prefix to cut on; it used to read as malformed.
func TestGetBranchUpstream_LocalRemote(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feat")
	temp_repo.RunGit(t, dir, "branch", "--set-upstream-to=main", "feat")

	remote, remoteBranch, err := git.Repo{Dir: dir}.GetBranchUpstream("feat")
	require.NoError(t, err)
	assert.Equal(t, remote, ".")
	assert.Equal(t, remoteBranch, "main")

	tracking, err := git.Repo{Dir: dir}.GetBranchTrackingRemote("feat")
	require.NoError(t, err)
	assert.Equal(t, tracking, ".")
}
