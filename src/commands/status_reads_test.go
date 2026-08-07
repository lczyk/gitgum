package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// Both default sections read the working tree, and a scan of a big one runs
// into seconds -- they share one rather than taking a scan each.
func TestStatusCommand_ScansOnce(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "# test repo\nedited\n")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute(nil), "status should succeed")

	assert.ContainsString(t, buf.String(), "README.md")
	assert.Equal(t, countCalls(calls(), "status"), 1)
	assert.Equal(t, countCalls(calls(), "diff"), 1)
}

// A section list that names neither changes nor head describes nothing in the
// working tree, so nothing should read it.
func TestStatusCommand_SectionsWithoutTreeReadsSkipScan(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"branch"}), "status branch should succeed")

	assert.Equal(t, countCalls(calls(), "status"), 0)
	assert.Equal(t, countCalls(calls(), "diff"), 0)
}

// --flat prints no diffstat, so it must not pay for the numstat that feeds one.
func TestStatusCommand_FlatSkipsNumstat(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "# test repo\nedited\n")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}, Flat: true}
	require.NoError(t, cmd.Execute([]string{"changes"}), "status changes --flat should succeed")

	assert.Equal(t, countCalls(calls(), "status"), 1)
	assert.Equal(t, countCalls(calls(), "diff"), 0)
}

// HEAD describes a ref, not the working tree. A status scan would answer it,
// but only after refreshing the index against every tracked file.
func TestStatusCommand_HeadAloneSkipsTheScan(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "untracked.txt", "hello\n")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"head"}), "status head should succeed")

	assert.ContainsString(t, buf.String(), "main")
	assert.Equal(t, countCalls(calls(), "status"), 0)
	assert.Equal(t, countCalls(calls(), "for-each-ref"), 1)
}

// The WORKTREES rows are decorated with a tracking remote and a subject each,
// which used to be a subprocess per row apiece.
func TestStatusCommand_WorktreesDecorateInOneRead(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "worktree", "add", t.TempDir(), "-b", "feat-a")
	temp_repo.RunGit(t, dir, "worktree", "add", t.TempDir(), "-b", "feat-b")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"worktree"}), "status worktree should succeed")

	assert.ContainsString(t, buf.String(), "feat-a")
	assert.ContainsString(t, buf.String(), "feat-b")
	assert.Equal(t, countCalls(calls(), "for-each-ref"), 1)
	assert.Equal(t, countCalls(calls(), "log"), 1)
}

// A detached HEAD reads the tracking remote of every containing branch. That
// is one ref listing, not one per branch -- three listings for the ref
// namespaces plus the one that answers tracking, whatever the branch count.
func TestStatusCommand_DetachedReadsTrackingOnce(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	for _, name := range []string{"one", "two", "three"} {
		temp_repo.RunGit(t, dir, "branch", name)
	}
	temp_repo.RunGit(t, dir, "checkout", "--detach")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"head"}), "status head should succeed")

	assert.ContainsString(t, buf.String(), "HEAD")
	assert.Equal(t, countCalls(calls(), "for-each-ref"), 4)
}

// CHANGES is the section that lists untracked files, so its scan looks for
// them -- and no longer asks for the branch line it has no use for.
func TestStatusCommand_ChangesScansUntrackedOnly(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "untracked.txt", "hello\n")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"changes"}), "status changes should succeed")

	assert.ContainsString(t, buf.String(), "untracked.txt")
	assert.Equal(t, countExact(calls(), "status", "--porcelain", "-z", "-unormal"), 1)
}

// The algorithm lookup is per repo, not per read -- it exists because the read
// env cannot see the setting, and paying for it on every read would undo the
// point of batching the reads in the first place.
func TestStatusCommand_ResolvesDiffAlgorithmOnce(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "# test repo\nedited\n")
	calls := gitShim(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute(nil), "status should succeed")

	assert.Equal(t, countExact(calls(), "config", "--get-regexp", "^diff\\."), 1)
}
