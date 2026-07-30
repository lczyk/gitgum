package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/pr"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// End-to-end Execute test: a sole remote is used without prompting, stub
// Selector picks the PR; bare repo has a synthetic refs/pull/1/head pointing at
// HEAD so ls-remote sees one PR.
func TestCheckoutPRCommand_Execute_ChecksOutPR(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)
	temp_repo.RunGit(t, dir, "push", "origin", "HEAD")

	// Simulate a GitHub-style PR ref on the bare remote.
	headSHA := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
	temp_repo.RunGit(t, bareDir, "update-ref", "refs/pull/1/head", headSHA)

	stub := &stubSelector{selectAnswers: []string{"PR #1 (head)"}}
	cmd := &CheckoutPRCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "pr/origin/1")
	// Sole remote skips the picker; only the PR selection prompts.
	assert.Equal(t, len(stub.selectCalls), 1)
	assert.ContainsString(t, stub.selectCalls[0].Prompt, "pull request")

	// PR identity is recorded on the branch so `gg pull` can update it.
	meta, ok, err := pr.ReadMeta(git.Repo{Dir: dir}, "pr/origin/1")
	require.NoError(t, err)
	require.That(t, ok, "PR metadata should be recorded")
	assert.Equal(t, meta.Remote, "origin")
	assert.Equal(t, meta.Number, 1)
	assert.Equal(t, meta.Type, "head")
}

// With more than one remote, the picker fires and its answer selects which
// remote to fetch the PR from.
func TestCheckoutPRCommand_Execute_MultipleRemotesPrompts(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "upstream", bareDir)
	temp_repo.RunGit(t, dir, "remote", "add", "fork", t.TempDir())
	temp_repo.RunGit(t, dir, "push", "upstream", "HEAD")

	headSHA := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
	temp_repo.RunGit(t, bareDir, "update-ref", "refs/pull/1/head", headSHA)

	stub := &stubSelector{selectAnswers: []string{"upstream", "PR #1 (head)"}}
	cmd := &CheckoutPRCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	require.NoError(t, err)
	assert.Equal(t, currentBranchIn(t, dir), "pr/upstream/1")
	assert.Equal(t, len(stub.selectCalls), 2)
	assert.ContainsString(t, stub.selectCalls[0].Prompt, "remote")
	assert.ContainsString(t, stub.selectCalls[1].Prompt, "pull request")
}
