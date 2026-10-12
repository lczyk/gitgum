package doctor

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestCheckReservedRemoteNames(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "local", "https://github.com/lczyk/gitgum")

	findings := checkReservedRemoteNames(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1)
	assert.Equal(t, findings[0].Check, "reserved-remote-name")
	assert.Equal(t, findings[0].Severity, SevWarning)
	assert.EqualArrays(t, findings[0].Fixes, []string{"git remote rename local lczyk"})
}

func TestCheckReservedRemoteNames_UnknownForge(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "local", "git@example.com:team/gitgum.git")

	findings := checkReservedRemoteNames(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1)
	assert.EqualArrays(t, findings[0].Fixes, []string{"git remote rename local <name>"})
}

func TestCheckReservedRemoteNames_Clean(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")

	assert.Equal(t, len(checkReservedRemoteNames(newFacts(git.Repo{Dir: dir}))), 0)
}
