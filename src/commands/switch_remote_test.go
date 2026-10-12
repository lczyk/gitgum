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

// divergedOtherRemote builds the same-name-on-two-remotes case: local main
// tracks origin/main and matches it, while a second remote "playground" has a
// main that shares history with origin's but has gone its own way (each side
// one commit the other lacks).
func divergedOtherRemote(t *testing.T) (local string) {
	t.Helper()
	local, origin := temp_repo.NewRepoWithRemote(t)
	playground := t.TempDir()
	temp_repo.RunGit(t, playground, "clone", "--bare", origin, ".")

	advanceRemote(t, origin, "origin.txt", "o", "feat: origin commit")
	temp_repo.RunGit(t, local, "pull", "--ff-only")
	advanceRemote(t, playground, "playground.txt", "p", "feat: playground commit")

	temp_repo.RunGit(t, local, "remote", "add", "playground", playground)
	temp_repo.RunGit(t, local, "fetch", "playground")
	return local
}

// Picking the same-name branch on another remote re-points tracking at it;
// when the integration goes through, the retarget stays.
func TestApplySelection_RemoteRetargetKeptOnSuccess(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)

	var out, errb strings.Builder
	stub := &stubSelector{selectAnswers: []string{git.PullMerge.String()}}
	s := &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: local}}}
	require.NoError(t, s.applySelection("remote: playground/main"))

	assert.Equal(t, upstreamOf(t, local, "main"), "playground/main")
	assert.ContainsString(t, out.String(), "Set tracking reference for local branch 'main' to remote branch 'playground/main'.")
	assert.ContainsString(t, out.String(), "Pulled 'playground/main' into 'main' (merge).")
	assert.That(t, !strings.Contains(errb.String(), "Restored the tracking reference"), "unexpected: ", errb.String())
}

// Backing out of the integration picker is a cancel, and a cancelled switch
// must not have moved the branch's upstream.
func TestApplySelection_RemoteRetargetUndoneOnCancel(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main"))

	var out, errb strings.Builder
	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	s := &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: local}}}
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main")), before)
	assert.ContainsString(t, errb.String(), "Restored the tracking reference of 'main' to 'origin/main'.")
}

// A fast-forward of a diverged branch cannot succeed; when git refuses it the
// failure is reported and tracking goes back to where it was, rather than
// leaving the branch tracking a remote whose history it doesn't carry.
func TestApplySelection_RemoteRetargetUndoneOnIntegrateFailure(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)

	var out, errb strings.Builder
	stub := &stubSelector{selectAnswers: []string{git.PullFFOnly.String()}}
	s := &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: local}}}
	err := s.applySelection("remote: playground/main")
	require.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "git merge")

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.Equal(t, currentBranchIn(t, local), "main")
	assert.ContainsString(t, errb.String(), "Restored the tracking reference of 'main' to 'origin/main'.")
}

// A branch that had no upstream before the retarget ends up with none again.
func TestApplySelection_RemoteRetargetUndoneToNoUpstream(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	temp_repo.RunGit(t, local, "branch", "--unset-upstream", "main")

	var out, errb strings.Builder
	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	s := &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: local}}}
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	// upstreamOf would fatal here: @{u} has nothing to resolve to.
	none := strings.TrimSpace(temp_repo.RunGit(t, local, "for-each-ref", "--format=%(upstream:short)", "refs/heads/main"))
	assert.Equal(t, none, "")
	assert.ContainsString(t, errb.String(), "Restored the tracking reference of 'main' to no upstream.")
}

// Nothing to undo when the local branch already tracks the picked remote: the
// pull runs as before and a cancel there leaves tracking alone.
func TestApplySelection_RemoteAlreadyTrackingNoRetarget(t *testing.T) {
	t.Parallel()
	local, origin := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, origin, "origin.txt", "o", "feat: origin commit")
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit")

	var out, errb strings.Builder
	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	s := &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: local}}}
	err := s.applySelection("remote: origin/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.That(t, !strings.Contains(out.String(), "Set tracking reference"), "unexpected: ", out.String())
	assert.That(t, !strings.Contains(errb.String(), "Restored the tracking reference"), "unexpected: ", errb.String())
}

func TestUpstreamShort(t *testing.T) {
	t.Parallel()
	assert.Equal(t, upstreamShort("origin", "refs/heads/main"), "origin/main")
	assert.Equal(t, upstreamShort("origin", "refs/heads/feat/x"), "origin/feat/x")
	assert.Equal(t, upstreamShort(".", "refs/heads/main"), "main")
}
