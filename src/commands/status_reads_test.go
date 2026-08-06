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
