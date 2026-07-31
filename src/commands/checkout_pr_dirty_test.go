package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
)

// prRepoWithStaleBranch builds the state the reset path needs: a local
// pr/origin/1 branch sitting one commit behind the PR head on the remote.
func prRepoWithStaleBranch(t *testing.T) string {
	t.Helper()
	dir := temp_repo.NewRepo(t)

	bareDir := t.TempDir()
	temp_repo.RunGit(t, bareDir, "init", "--bare")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", bareDir)
	temp_repo.RunGit(t, dir, "push", "origin", "HEAD")

	base := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))

	// Advance the PR head by one commit, then rewind the local side so the
	// branch created below is genuinely stale.
	temp_repo.CreateCommit(t, dir, "pr.txt", "from the PR\n", "feat: pr work")
	temp_repo.RunGit(t, dir, "push", "origin", "HEAD:refs/pull/1/head")
	temp_repo.RunGit(t, dir, "reset", "--hard", base)

	// No PR metadata on it: the reset path keys off the branch existing, and
	// leaving it off also covers a branch made before that metadata existed.
	temp_repo.RunGit(t, dir, "branch", "pr/origin/1", base)
	return dir
}

// TestCheckoutPRCommand_ResetExistingBranch_StashesDirtyTree pins the reset
// path to the same working-tree guard the fresh-branch path uses. It used to
// run `reset --hard` straight after the checkout, so uncommitted tracked
// changes were destroyed with no prompt at all.
func TestCheckoutPRCommand_ResetExistingBranch_StashesDirtyTree(t *testing.T) {
	t.Parallel()
	dir := prRepoWithStaleBranch(t)
	temp_repo.WriteFile(t, dir, "README.md", "local edit worth keeping\n")

	var out bytes.Buffer
	// Reset the branch: yes. Then pick the stash row on the dirty prompt.
	stub := &stubSelector{
		selectAnswers:  []string{"PR #1 (head)", dirtyStashOption("checkout-pr")},
		confirmAnswers: []bool{true},
	}
	cmd := &CheckoutPRCommand{cmdIO: cmdIO{Out: &out, UI: stub, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute(nil))

	// The dirty-tree prompt fired, naming this subcommand.
	require.Equal(t, len(stub.selectCalls), 2)
	assert.ContainsString(t, stub.selectCalls[1].Options[1], "run checkout-pr")

	// The reset landed...
	assert.Equal(t, currentBranchIn(t, dir), "pr/origin/1")
	fileExists(t, dir, "pr.txt")
	// ...and the uncommitted work came back with it.
	fileContent(t, dir, "README.md", "local edit worth keeping\n")
}

// Declining the stash aborts before anything destructive runs: the branch stays
// where it was and the working tree is untouched.
func TestCheckoutPRCommand_ResetExistingBranch_DeclineStashAborts(t *testing.T) {
	t.Parallel()
	dir := prRepoWithStaleBranch(t)
	temp_repo.WriteFile(t, dir, "README.md", "local edit worth keeping\n")

	var out bytes.Buffer
	// Reset the branch: yes. Then pick abort on the dirty prompt.
	stub := &stubSelector{
		selectAnswers:  []string{"PR #1 (head)", dirtyAbort},
		confirmAnswers: []bool{true},
	}
	cmd := &CheckoutPRCommand{cmdIO: cmdIO{Out: &out, UI: stub, Repo: git.Repo{Dir: dir}}}
	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled, "aborting stops the command")

	fileNotExists(t, dir, "pr.txt") // no reset happened
	fileContent(t, dir, "README.md", "local edit worth keeping\n")
}
