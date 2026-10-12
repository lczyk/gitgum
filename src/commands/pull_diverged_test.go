package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
)

// divergedWithOrigin leaves local main one commit ahead of and one behind
// origin/main, and returns the remote's tip -- read from the remote, since the
// local origin/main ref is stale until the pull fetches. With conflicting, both
// commits write the same file.
func divergedWithOrigin(t *testing.T, conflicting bool) (local, remoteTip string) {
	t.Helper()
	local, remote := temp_repo.NewRepoWithRemote(t)
	remoteFile, localFile := "remote.txt", "local.txt"
	if conflicting {
		remoteFile, localFile = "shared.txt", "shared.txt"
	}
	remoteTip = advanceRemote(t, remote, remoteFile, "r", "feat: remote commit")
	temp_repo.CreateCommit(t, local, localFile, "l", "feat: local commit")
	return local, remoteTip
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
}

// The picker only ever appears for a diverged branch, where a fast-forward
// cannot succeed, so that row is not offered; what is offered reconciles or
// steps aside.
func TestPullCommand_DivergedPickerRows(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, false)

	stub := &stubSelector{selectAnswers: []string{pullLeaveLabel}}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}
	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, stub.selectCalls[0].Prompt, "Integrate 'origin/main' into 'local/main' via")
	assert.EqualArrays(t, stub.selectCalls[0].Options, []string{
		"rebase", "merge", "reset to 'origin/main' (drop 1 local commit)", "leave as is"})
}

// "leave as is" is a decline: nothing moves, the command says what was left
// undone, exit 0.
func TestPullCommand_LeaveAsIs(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, false)
	before := headOf(t, local)

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{pullLeaveLabel}}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}
	require.NoError(t, cmd.Execute(nil))

	assert.Equal(t, headOf(t, local), before)
	assert.ContainsString(t, buf.String(), "Left 'local/main' as it is: ahead 1, behind 1 of 'origin/main'.")
}

// Reset lands the branch on the upstream tip, dropping the local commit, after
// a confirmation that defaults to no.
func TestPullCommand_ResetToUpstream(t *testing.T) {
	t.Parallel()
	local, remoteTip := divergedWithOrigin(t, false)

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{"reset to 'origin/main' (drop 1 local commit)"}, confirmAnswers: []bool{true}}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}
	require.NoError(t, cmd.Execute(nil))

	require.Equal(t, len(stub.confirmCalls), 1)
	assert.Equal(t, stub.confirmCalls[0].Prompt, "Reset 'local/main' to 'origin/main', dropping 1 local commit?")
	assert.That(t, !stub.confirmCalls[0].DefaultYes, "reset must default to no")
	assert.Equal(t, headOf(t, local), remoteTip)
	assert.ContainsString(t, buf.String(), "Pulled 'origin/main' into 'local/main' (reset to upstream).")
}

// Declining the reset confirmation is "leave as is".
func TestPullCommand_ResetDeclined(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, false)
	before := headOf(t, local)

	var buf strings.Builder
	stub := &stubSelector{selectAnswers: []string{"reset to 'origin/main' (drop 1 local commit)"}, confirmAnswers: []bool{false}}
	cmd := &PullCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}
	require.NoError(t, cmd.Execute(nil))

	assert.Equal(t, headOf(t, local), before)
	assert.ContainsString(t, buf.String(), "Left 'local/main' as it is")
}

// Esc in the picker is a cancel, with nothing changed.
func TestPullCommand_PickerCancelled(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, false)
	before := headOf(t, local)

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	cmd := &PullCommand{cmdIO: cmdIO{UI: stub, Repo: git.Repo{Dir: local}}}
	assert.ErrorIs(t, cmd.Execute(nil), ui.ErrCancelled)
	assert.Equal(t, headOf(t, local), before)
}

// A merge that conflicts is aborted rather than left for the user to sort out:
// the pull failed, and the branch is as it was.
func TestPullCommand_MergeConflictAborted(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, true)
	before := headOf(t, local)

	var errBuf strings.Builder
	stub := &stubSelector{selectAnswers: []string{git.PullMerge.String()}}
	cmd := &PullCommand{cmdIO: cmdIO{Err: &errBuf, UI: stub, Repo: git.Repo{Dir: local}}}
	err := cmd.Execute(nil)
	require.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "merge")

	_, inProgress := git.Repo{Dir: local}.InProgress()
	assert.That(t, !inProgress, "merge must have been aborted")
	assert.Equal(t, headOf(t, local), before)
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "status", "--porcelain")), "")
	assert.ContainsString(t, errBuf.String(), "The merge stopped part-way; aborted it, so 'local/main' is as it was.")
}

// Likewise a rebase that stops on a conflict.
func TestPullCommand_RebaseConflictAborted(t *testing.T) {
	t.Parallel()
	local, _ := divergedWithOrigin(t, true)
	before := headOf(t, local)

	var errBuf strings.Builder
	stub := &stubSelector{selectAnswers: []string{git.PullRebase.String()}}
	cmd := &PullCommand{cmdIO: cmdIO{Err: &errBuf, UI: stub, Repo: git.Repo{Dir: local}}}
	err := cmd.Execute(nil)
	require.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "rebase")

	_, inProgress := git.Repo{Dir: local}.InProgress()
	assert.That(t, !inProgress, "rebase must have been aborted")
	assert.Equal(t, headOf(t, local), before)
	assert.Equal(t, currentBranchIn(t, local), "main")
	assert.ContainsString(t, errBuf.String(), "The rebase stopped part-way; aborted it, so 'local/main' is as it was.")
}
