package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestPRBranchNameAndRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, prBranchName("origin", 51), "pr/origin/51")
	assert.Equal(t, prBranchName("upstream", 7), "pr/upstream/7")
	assert.Equal(t, prMeta{number: 51, typ: "head"}.ref(), "refs/pull/51/head")
	assert.Equal(t, prMeta{number: 9, typ: "merge"}.ref(), "refs/pull/9/merge")
}

func TestReadPRMeta_FromConfig(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	want := prMeta{remote: "upstream", number: 72, typ: "merge"}
	require.NoError(t, writePRMeta(r, "pr/upstream/72", want))

	got, ok, err := readPRMeta(r, "pr/upstream/72")
	require.NoError(t, err)
	require.That(t, ok, "config-backed PR meta should be found")
	assert.Equal(t, got, want)
}

func TestReadPRMeta_NameFallback(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	// No config written -- identity derived from the pr/<remote>/<number> name,
	// defaulting to the head ref.
	got, ok, err := readPRMeta(r, "pr/origin/5")
	require.NoError(t, err)
	require.That(t, ok, "name should be parsed as a fallback")
	assert.Equal(t, got, prMeta{remote: "origin", number: 5, typ: "head"})
}

func TestReadPRMeta_NotAPRBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	for _, b := range []string{"main", "feat/foo", "pr/origin/notanumber", "pr/origin"} {
		_, ok, err := readPRMeta(r, b)
		require.NoError(t, err)
		assert.That(t, !ok, "%q should not be a PR branch", b)
	}
}

func TestSwitchUnselectable(t *testing.T) {
	t.Parallel()
	// current branch (HEAD row): selectable now (picking it pulls).
	assert.That(t, !switchUnselectable("local: main"+currentBranchMarker), "current branch should be selectable")
	// branch checked out in another worktree: still blocked.
	assert.That(t, switchUnselectable("local: feat"+checkedOutSuffix("/wt/feat-2")), "other-worktree checkout stays blocked")
	// detached HEAD row: still blocked.
	assert.That(t, switchUnselectable(detachedEntry("cf10b5f")), "detached HEAD stays blocked")
	// a plain branch: selectable.
	assert.That(t, !switchUnselectable("local: other"), "plain branch selectable")
}
