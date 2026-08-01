package git_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestGetDefaultBranch_LocalMainFallback(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	// NewRepo's initial branch is whatever git's init.defaultBranch is set to.
	// Force a known default so the assertion isn't environment-dependent.
	temp_repo.RunGit(t, dir, "branch", "-m", "main")

	got, err := git.Repo{Dir: dir}.GetDefaultBranch()
	require.NoError(t, err)
	assert.Equal(t, got, "main")
}

func TestGetDefaultBranch_LocalMaster(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "-m", "master")

	got, err := git.Repo{Dir: dir}.GetDefaultBranch()
	require.NoError(t, err)
	assert.Equal(t, got, "master")
}

func TestGetDefaultBranch_FromRemoteHEAD(t *testing.T) {
	t.Parallel()
	upstream := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, upstream, "branch", "-m", "trunk")

	dir := temp_repo.NewRepo(t)
	// Local default branch isn't named trunk — origin's HEAD must take
	// precedence over the local-fallback heuristic.
	temp_repo.RunGit(t, dir, "branch", "-m", "main")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", upstream)
	temp_repo.RunGit(t, dir, "fetch", "origin")
	temp_repo.RunGit(t, dir, "remote", "set-head", "origin", "trunk")

	got, err := git.Repo{Dir: dir}.GetDefaultBranch()
	require.NoError(t, err)
	assert.Equal(t, got, "trunk")
}

func TestDirtyTracked_Clean(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	entries, err := git.Repo{Dir: dir}.DirtyTracked()
	require.NoError(t, err)
	assert.Equal(t, len(entries), 0)
}

func TestDirtyTracked_FiltersUntracked(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	require.NoError(t, err)

	entries, err := git.Repo{Dir: dir}.DirtyTracked()
	require.NoError(t, err)
	assert.Equal(t, len(entries), 0)
}

// Mixed tracked + untracked: only tracked entries come back, with the unstaged
// half reported on Y and X left blank.
func TestDirtyTracked_KeepsBothStatusSides(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	require.NoError(t, err)

	entries, err := git.Repo{Dir: dir}.DirtyTracked()
	require.NoError(t, err)
	require.Equal(t, len(entries), 1)
	assert.Equal(t, entries[0].Path, "README.md")
	assert.Equal(t, entries[0].X, byte(' '))
	assert.Equal(t, entries[0].Y, byte('M'))
}

func TestDirtyTracked_StagedAndUnstaged(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("s\n"), 0o644)
	require.NoError(t, err)
	temp_repo.RunGit(t, dir, "add", "staged.txt")
	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("m\n"), 0o644)
	require.NoError(t, err)

	entries, err := git.Repo{Dir: dir}.DirtyTracked()
	require.NoError(t, err)
	assert.That(t, slices.ContainsFunc(entries, func(e git.Entry) bool {
		return e.Path == "staged.txt" && e.X == 'A' && e.Y == ' '
	}), "staged file present with A status")
	assert.That(t, slices.ContainsFunc(entries, func(e git.Entry) bool {
		return e.Path == "README.md" && e.X == ' ' && e.Y == 'M'
	}), "modified file present with unstaged status")
}

func TestStashPush_AndPopIndex_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644)
	require.NoError(t, err)

	err = r.StashPush("test stash")
	require.NoError(t, err)

	// Tree clean, stash list contains entry.
	clean := strings.TrimSpace(temp_repo.RunGit(t, dir, "status", "--porcelain"))
	assert.Equal(t, clean, "")
	assert.ContainsString(t, temp_repo.RunGit(t, dir, "stash", "list"), "test stash")

	err = r.StashPopIndex()
	require.NoError(t, err)

	// Modification restored, stash list empty.
	entries, err := r.DirtyTracked()
	require.NoError(t, err)
	assert.That(t, slices.ContainsFunc(entries, func(e git.Entry) bool {
		return e.Path == "README.md" && e.Y == 'M'
	}), "modification restored after pop")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}

func TestStashPopIndex_NoStash(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := git.Repo{Dir: dir}.StashPopIndex()
	assert.Error(t, err, assert.AnyError, "pop with empty stash list should error")
}

func TestInProgress_CleanRepo(t *testing.T) {
	t.Parallel()
	r := git.Repo{Dir: temp_repo.NewRepo(t)}
	operation, yes := r.InProgress()
	assert.That(t, !yes, "an ordinary checkout is not mid-operation")
	assert.Equal(t, operation, "")
}

// Dirty but not mid-operation: having edits is exactly the state discarding is
// for, so it must not be mistaken for one.
func TestInProgress_DirtyButNotMidOperation(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "changed\n")
	_, yes := git.Repo{Dir: dir}.InProgress()
	assert.That(t, !yes, "uncommitted changes alone are not an operation")
}

func TestInProgress_ConflictedMerge(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	temp_repo.WriteFile(t, dir, "README.md", "from other\n")
	temp_repo.RunGit(t, dir, "add", "README.md")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: other side")

	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.WriteFile(t, dir, "README.md", "from main\n")
	temp_repo.RunGit(t, dir, "add", "README.md")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: main side")

	out, err := temp_repo.RunGitAllowFail(t, dir, "merge", "other")
	require.That(t, err != nil, "the merge should have conflicted: %s", out)

	operation, yes := git.Repo{Dir: dir}.InProgress()
	require.That(t, yes, "a stopped merge should be detected")
	assert.ContainsString(t, operation, "merge")
}

func TestInProgress_StoppedRebase(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	temp_repo.WriteFile(t, dir, "README.md", "from other\n")
	temp_repo.RunGit(t, dir, "add", "README.md")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: other side")

	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.WriteFile(t, dir, "README.md", "from main\n")
	temp_repo.RunGit(t, dir, "add", "README.md")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: main side")

	out, err := temp_repo.RunGitAllowFail(t, dir, "rebase", "other")
	require.That(t, err != nil, "the rebase should have conflicted: %s", out)

	operation, yes := git.Repo{Dir: dir}.InProgress()
	require.That(t, yes, "a stopped rebase should be detected")
	assert.ContainsString(t, operation, "rebase")
}
