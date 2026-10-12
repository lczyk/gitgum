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

// otherRemote builds the same-name-on-two-remotes case: local main tracks
// origin/main and matches it, while a second remote "playground" has a main
// with one commit of its own. With originAhead, origin has one too, so the two
// mains have diverged; with conflicting, both commits touch the same file the
// same way a merge cannot resolve.
func otherRemote(t *testing.T, originAhead, conflicting bool) (local string) {
	t.Helper()
	local, origin := temp_repo.NewRepoWithRemote(t)
	playground := t.TempDir()
	temp_repo.RunGit(t, playground, "clone", "--bare", origin, ".")

	file := "playground.txt"
	if conflicting {
		file = "shared.txt"
	}
	if originAhead {
		originFile := "origin.txt"
		if conflicting {
			originFile = "shared.txt"
		}
		advanceRemote(t, origin, originFile, "o", "feat: origin commit")
		temp_repo.RunGit(t, local, "pull", "--ff-only")
	}
	advanceRemote(t, playground, file, "p", "feat: playground commit")

	temp_repo.RunGit(t, local, "remote", "add", "playground", playground)
	temp_repo.RunGit(t, local, "fetch", "playground")
	return local
}

func divergedOtherRemote(t *testing.T) string { return otherRemote(t, true, false) }

// upstreamOrNone is upstreamOf for a branch that may have no upstream, which
// rev-parse @{u} would fatal on.
func upstreamOrNone(t *testing.T, dir, branch string) string {
	t.Helper()
	return strings.TrimSpace(temp_repo.RunGit(t, dir, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch))
}

func newSwitch(dir string, stub *stubSelector) (*SwitchCommand, *strings.Builder, *strings.Builder) {
	var out, errb strings.Builder
	return &SwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errb, UI: stub, Repo: git.Repo{Dir: dir}}}, &out, &errb
}

// Diverged: the retarget is a question. Confirmed, and with the integration
// going through, it stays.
func TestApplySelection_RemoteRetargetConfirmedKeptOnSuccess(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{git.PullMerge.String()}}
	s, out, errb := newSwitch(local, stub)
	require.NoError(t, s.applySelection("remote: playground/main"))

	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "Retarget tracking of 'local/main' to 'playground/main'?")
	assert.That(t, !stub.confirmCalls[0].DefaultYes, "retarget must default to no")
	assert.ContainsString(t, out.String(), "'local/main' tracks 'origin/main' and has diverged from 'playground/main' (ahead 1, behind 1).")
	assert.Equal(t, upstreamOf(t, local, "main"), "playground/main")
	assert.ContainsString(t, out.String(), "Set tracking of 'local/main' to 'playground/main'.")
	assert.ContainsString(t, out.String(), "Pulled 'playground/main' into 'local/main' (merge).")
	assert.That(t, !strings.Contains(errb.String(), "Restored the tracking"), "unexpected: ", errb.String())
}

// Declining the retarget is a decline, not a cancel: the switch still lands on
// the branch, tracking untouched, and pulls from where it always did.
func TestApplySelection_RemoteRetargetDeclinedLeavesTracking(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)

	stub := &stubSelector{confirmAnswers: []bool{false}}
	s, out, _ := newSwitch(local, stub)
	require.NoError(t, s.applySelection("remote: playground/main"))

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.Equal(t, currentBranchIn(t, local), "main")
	assert.ContainsString(t, out.String(), "Switched to 'local/main', tracking left as it was.")
	assert.ContainsString(t, out.String(), "Already up to date. 'local/main' matches 'origin/main'.")
	assert.That(t, !strings.Contains(out.String(), "Set tracking"), "unexpected: ", out.String())
}

// Not diverged (the other remote is strictly ahead): no question, the retarget
// happens and the branch fast-forwards onto it.
func TestApplySelection_RemoteRetargetWithoutAskingWhenNotDiverged(t *testing.T) {
	t.Parallel()
	local := otherRemote(t, false, false)
	tip := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "playground/main"))

	stub := &stubSelector{}
	s, out, _ := newSwitch(local, stub)
	require.NoError(t, s.applySelection("remote: playground/main"))

	assert.Equal(t, len(stub.confirmCalls), 0)
	assert.Equal(t, upstreamOf(t, local, "main"), "playground/main")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main")), tip)
	assert.ContainsString(t, out.String(), "Pulled 'playground/main' into 'local/main' (fast-forward only).")
}

// Backing out of the integration picker is a cancel, and a cancelled switch
// must not have moved the branch's upstream.
func TestApplySelection_RemoteRetargetUndoneOnCancel(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main"))

	stub := &stubSelector{confirmAnswers: []bool{true}, selectErrs: []error{ui.ErrCancelled}}
	s, _, errb := newSwitch(local, stub)
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main")), before)
	assert.ContainsString(t, errb.String(), "Restored the tracking of 'local/main' to 'origin/main'.")
}

// A merge that conflicts is aborted and reported, and the tracking goes back to
// where it was rather than staying on a remote whose history the branch doesn't
// carry.
func TestApplySelection_RemoteRetargetUndoneOnIntegrateFailure(t *testing.T) {
	t.Parallel()
	local := otherRemote(t, true, true)
	before := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main"))

	stub := &stubSelector{confirmAnswers: []bool{true}, selectAnswers: []string{git.PullMerge.String()}}
	s, _, errb := newSwitch(local, stub)
	err := s.applySelection("remote: playground/main")
	require.Error(t, err, assert.AnyError)
	assert.ContainsString(t, err.Error(), "git merge")

	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.Equal(t, currentBranchIn(t, local), "main")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "main")), before)
	_, inProgress := git.Repo{Dir: local}.InProgress()
	assert.That(t, !inProgress, "the conflicted merge must have been aborted")
	assert.ContainsString(t, errb.String(), "The merge stopped part-way; aborted it, so 'local/main' is as it was.")
	assert.ContainsString(t, errb.String(), "Restored the tracking of 'local/main' to 'origin/main'.")
}

// A branch that had no upstream before the retarget ends up with none again.
func TestApplySelection_RemoteRetargetUndoneToNoUpstream(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	temp_repo.RunGit(t, local, "branch", "--unset-upstream", "main")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectErrs: []error{ui.ErrCancelled}}
	s, out, errb := newSwitch(local, stub)
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.ContainsString(t, out.String(), "'local/main' tracks no upstream and has diverged")
	assert.Equal(t, upstreamOrNone(t, local, "main"), "")
	assert.ContainsString(t, errb.String(), "Restored the tracking of 'local/main' to no upstream.")
}

// Nothing to undo when the local branch already tracks the picked remote: the
// pull runs as before and a cancel there leaves tracking alone.
func TestApplySelection_RemoteAlreadyTrackingNoRetarget(t *testing.T) {
	t.Parallel()
	local, origin := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, origin, "origin.txt", "o", "feat: origin commit")
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit")

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	s, out, errb := newSwitch(local, stub)
	err := s.applySelection("remote: origin/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, len(stub.confirmCalls), 0)
	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	assert.That(t, !strings.Contains(out.String(), "Set tracking"), "unexpected: ", out.String())
	assert.That(t, !strings.Contains(errb.String(), "Restored the tracking"), "unexpected: ", errb.String())
}

// A switch that is cancelled after the checkout goes back to the branch it
// started from -- the checkout was a step towards a goal that was not reached.
func TestApplySelection_BackOnPreviousBranchOnCancel(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	temp_repo.RunGit(t, local, "checkout", "-b", "feature")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectErrs: []error{ui.ErrCancelled}}
	s, out, errb := newSwitch(local, stub)
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.ContainsString(t, out.String(), "Switched to 'local/main'.")
	assert.Equal(t, currentBranchIn(t, local), "feature")
	assert.Equal(t, upstreamOf(t, local, "main"), "origin/main")
	// undone in reverse: the tracking first, then the checkout.
	restored := strings.Index(errb.String(), "Restored the tracking")
	back := strings.Index(errb.String(), "Back on 'local/feature'.")
	assert.That(t, restored >= 0 && back > restored, "expected tracking restore before going back, got: ", errb.String())
}

// The same from a detached HEAD: back goes to the commit, named by its short
// sha, since there is no branch to go back to.
func TestApplySelection_BackOnDetachedCommitOnCancel(t *testing.T) {
	t.Parallel()
	local := divergedOtherRemote(t)
	sha := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD"))
	temp_repo.RunGit(t, local, "checkout", "--detach")

	stub := &stubSelector{confirmAnswers: []bool{true}, selectErrs: []error{ui.ErrCancelled}}
	s, _, errb := newSwitch(local, stub)
	err := s.applySelection("remote: playground/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, currentBranchIn(t, local), "HEAD")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "HEAD")), sha)
	assert.ContainsString(t, errb.String(), "Back on '"+sha[:7]+"'.")
}

// Landing on a local branch and cancelling its pull likewise goes back.
func TestApplySelection_LocalBackOnCancel(t *testing.T) {
	t.Parallel()
	local, origin := temp_repo.NewRepoWithRemote(t)
	advanceRemote(t, origin, "origin.txt", "o", "feat: origin commit")
	temp_repo.CreateCommit(t, local, "local.txt", "l", "feat: local commit") // main diverged from origin/main
	temp_repo.RunGit(t, local, "checkout", "-b", "feature")

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	s, _, errb := newSwitch(local, stub)
	err := s.applySelection("local/remote: origin/main")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.Equal(t, currentBranchIn(t, local), "feature")
	assert.ContainsString(t, errb.String(), "Back on 'local/feature'.")
}

// The create path: declining the creation is a cancel before anything is done,
// so it costs nothing and creates nothing.
func TestApplySelection_RemoteCreateDeclined(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "update-ref", "refs/remotes/origin/topic", "HEAD")

	stub := &stubSelector{confirmAnswers: []bool{false}}
	s, _, errb := newSwitch(local, stub)
	err := s.applySelection("remote: origin/topic")
	assert.ErrorIs(t, err, ui.ErrCancelled)

	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "No 'local/topic' yet. Create it tracking 'origin/topic'?")
	assert.That(t, !git.Repo{Dir: local}.BranchExists("topic"), "no branch must have been created")
	assert.ContainsString(t, errb.String(), "Not creating a local tracking branch. Aborting switch.")
}

func TestApplySelection_RemoteCreateLands(t *testing.T) {
	t.Parallel()
	local, origin := temp_repo.NewRepoWithRemote(t)
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", origin, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Other User")
	temp_repo.RunGit(t, other, "config", "user.email", "other@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.RunGit(t, other, "checkout", "-b", "topic")
	temp_repo.CreateCommit(t, other, "topic.txt", "t", "feat: topic")
	temp_repo.RunGit(t, other, "push", "origin", "topic")
	temp_repo.RunGit(t, local, "fetch", "origin")

	stub := &stubSelector{confirmAnswers: []bool{true}}
	s, out, _ := newSwitch(local, stub)
	require.NoError(t, s.applySelection("remote: origin/topic"))

	assert.Equal(t, currentBranchIn(t, local), "topic")
	assert.Equal(t, upstreamOf(t, local, "topic"), "origin/topic")
	assert.ContainsString(t, out.String(), "Created and switched to 'local/topic' tracking 'origin/topic'.")
}

func TestUpstreamShort(t *testing.T) {
	t.Parallel()
	assert.Equal(t, upstreamShort("origin", "refs/heads/main"), "origin/main")
	assert.Equal(t, upstreamShort("origin", "refs/heads/feat/x"), "origin/feat/x")
	assert.Equal(t, upstreamShort(".", "refs/heads/main"), "local/main")
}
