package git_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert"
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
	assert.NoError(t, err)
	assert.Equal(t, got, "main")
}

func TestGetDefaultBranch_LocalMaster(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "-m", "master")

	got, err := git.Repo{Dir: dir}.GetDefaultBranch()
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, got, "trunk")
}

func TestDirtyTrackedLines_Clean(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	lines, err := git.Repo{Dir: dir}.DirtyTrackedLines()
	assert.NoError(t, err)
	assert.Equal(t, len(lines), 0)
}

func TestDirtyTrackedLines_FiltersUntracked(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	assert.NoError(t, err)

	lines, err := git.Repo{Dir: dir}.DirtyTrackedLines()
	assert.NoError(t, err)
	assert.Equal(t, len(lines), 0)
}

// Mixed tracked + untracked: only tracked lines come back, with their leading
// porcelain XY codes intact (regression for the TrimSpace bug that ate leading
// spaces from " M file" entries).
func TestDirtyTrackedLines_PreservesLeadingSpace(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	assert.NoError(t, err)

	lines, err := git.Repo{Dir: dir}.DirtyTrackedLines()
	assert.NoError(t, err)
	assert.Equal(t, len(lines), 1)
	assert.Equal(t, lines[0], " M README.md")
}

func TestDirtyTrackedLines_StagedAndUnstaged(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("s\n"), 0o644)
	assert.NoError(t, err)
	temp_repo.RunGit(t, dir, "add", "staged.txt")
	err = os.WriteFile(filepath.Join(dir, "README.md"), []byte("m\n"), 0o644)
	assert.NoError(t, err)

	lines, err := git.Repo{Dir: dir}.DirtyTrackedLines()
	assert.NoError(t, err)
	assert.That(t, slices.Contains(lines, "A  staged.txt"), "staged file present with A status")
	assert.That(t, slices.Contains(lines, " M README.md"), "modified file present with unstaged status")
}

func TestStashPush_AndPopIndex_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644)
	assert.NoError(t, err)

	err = r.StashPush("test stash")
	assert.NoError(t, err)

	// Tree clean, stash list contains entry.
	clean := strings.TrimSpace(temp_repo.RunGit(t, dir, "status", "--porcelain"))
	assert.Equal(t, clean, "")
	assert.ContainsString(t, temp_repo.RunGit(t, dir, "stash", "list"), "test stash")

	err = r.StashPopIndex()
	assert.NoError(t, err)

	// Modification restored, stash list empty.
	lines, err := r.DirtyTrackedLines()
	assert.NoError(t, err)
	assert.That(t, slices.Contains(lines, " M README.md"), "modification restored after pop")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}

func TestStashPopIndex_NoStash(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := git.Repo{Dir: dir}.StashPopIndex()
	assert.Error(t, err, assert.AnyError, "pop with empty stash list should error")
}
