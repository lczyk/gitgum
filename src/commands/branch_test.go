package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestBranchCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &BranchCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

func TestBranchCommand_CreatesOffPickedLocalBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"local: feature"},
		promptAnswers: []string{"feat/new"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, currentBranchIn(t, dir), "feat/new")
	assert.ContainsString(t, buf.String(), "Created and switched to branch 'feat/new' off 'feature'.")

	// The new branch starts where the picked one did, not where HEAD was.
	head := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "feat/new"))
	feature := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "feature"))
	assert.Equal(t, head, feature)
}

// Cutting a branch off a detached HEAD is how commits made there stop being
// unreachable, so `branch` offers HEAD as a start point -- unlike `switch`,
// which can only refuse to check it out.
func TestBranchCommand_CreatesOffDetachedHEAD(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a", "chore: second")
	temp_repo.RunGit(t, dir, "checkout", "--detach", "HEAD~1")
	temp_repo.CreateCommit(t, dir, "b.txt", "b", "feat: work done while detached")
	detached := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "HEAD"))
	short := detachedShortSHA(git.Repo{Dir: dir}, "")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"local: HEAD (detached at " + short + ")"},
		promptAnswers: []string{"feat/rescued"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, currentBranchIn(t, dir), "feat/rescued")
	assert.ContainsString(t, buf.String(), "Created and switched to branch 'feat/rescued' off '"+short+"'.")

	// the branch carries the once-orphaned commit
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "feat/rescued")), detached)
}

// The current branch is a valid start point -- and the most common one -- so it
// must be offered rather than filtered out the way switch does.
func TestBranchCommand_CreatesOffCurrentBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	current := currentBranchIn(t, dir)

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"local: " + current},
		promptAnswers: []string{"wip"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, currentBranchIn(t, dir), "wip")
}

func TestBranchCommand_PromptsForName(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")

	stub := &stubSelector{
		selectAnswers: []string{"local: feature"},
		promptAnswers: []string{"other"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	require.Equal(t, len(stub.promptCalls), 1)
	assert.Equal(t, stub.promptCalls[0].Prompt, "New branch name")
}

func TestBranchCommand_RejectsExistingName(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	current := currentBranchIn(t, dir)

	stub := &stubSelector{
		selectAnswers: []string{"local: " + current},
		promptAnswers: []string{"feature"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

	err := cmd.Execute(nil)
	assert.Error(t, err, assert.AnyError, "should refuse to clobber an existing branch")
	assert.ContainsString(t, err.Error(), "already exists")
	assert.Equal(t, currentBranchIn(t, dir), current)
}

// The prompt returns the query verbatim, so a bare Enter (and its whitespace
// cousin) reaches promptName and has to be rejected there.
func TestBranchCommand_RejectsEmptyName(t *testing.T) {
	t.Parallel()

	for name, answer := range map[string]string{"bare enter": "", "whitespace": "   "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := temp_repo.NewRepo(t)
			current := currentBranchIn(t, dir)

			stub := &stubSelector{
				selectAnswers: []string{"local: " + current},
				promptAnswers: []string{answer},
			}
			cmd := &BranchCommand{cmdIO: cmdIO{Out: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}}

			err := cmd.Execute(nil)
			assert.Error(t, err, assert.AnyError, "should refuse an empty name")
			assert.ContainsString(t, err.Error(), "empty branch name")
			assert.Equal(t, currentBranchIn(t, dir), current)
		})
	}
}

// Branching off a remote-tracking ref must not inherit it as upstream: the new
// branch is ours, and a later `gg push` would otherwise target someone else's.
func TestBranchCommand_RemoteStartPointDoesNotTrack(t *testing.T) {
	t.Parallel()
	local, _ := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "fetch", "origin")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"remote: origin/main"},
		promptAnswers: []string{"feat/off-remote"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: local}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, currentBranchIn(t, local), "feat/off-remote")

	upstream, err := git.Repo{Dir: local}.GetBranchTrackingRemote("feat/off-remote")
	require.NoError(t, err)
	assert.Equal(t, upstream, "")
}

// A remote start point is fetched first, same as switch does -- branching off a
// ref that silently lagged the remote by however long since the last fetch is
// the surprising behaviour.
func TestBranchCommand_RemoteStartPointIsFetchedFirst(t *testing.T) {
	t.Parallel()
	local, remote := temp_repo.NewRepoWithRemote(t)
	temp_repo.RunGit(t, local, "fetch", "origin")

	// Advance origin/main behind local's back, via a second clone.
	other := t.TempDir()
	temp_repo.RunGit(t, other, "clone", remote, ".")
	temp_repo.RunGit(t, other, "config", "user.name", "Test User")
	temp_repo.RunGit(t, other, "config", "user.email", "test@example.com")
	temp_repo.RunGit(t, other, "config", "commit.gpgsign", "false")
	// Escape the user's global hooks (a pre-push hook blocks agent pushes).
	temp_repo.RunGit(t, other, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, other, "new.txt", "new", "feat: land upstream")
	temp_repo.RunGit(t, other, "push", "origin", "main")
	upstreamTip := strings.TrimSpace(temp_repo.RunGit(t, other, "rev-parse", "HEAD"))

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"remote: origin/main"},
		promptAnswers: []string{"feat/fresh"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: local}}}

	require.NoError(t, cmd.Execute(nil))
	assert.ContainsString(t, buf.String(), "Fetching 'origin/main'...")

	// Without the fetch the new branch would sit on the pre-push tip.
	got := strings.TrimSpace(temp_repo.RunGit(t, local, "rev-parse", "feat/fresh"))
	assert.Equal(t, got, upstreamTip)
}

// A branch checked out in another worktree can't be switched to or deleted, but
// it's a perfectly good start point -- so the picker must leave it selectable
// and unmarked, and applying the selection must not choke on a display suffix.
func TestBranchCommand_CheckedOutElsewhereIsAValidStartPoint(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "branch", "feature")
	temp_repo.RunGit(t, dir, "worktree", "add", t.TempDir(), "feature")

	var buf strings.Builder
	stub := &stubSelector{
		selectAnswers: []string{"local: feature"},
		promptAnswers: []string{"feat/child"},
	}
	cmd := &BranchCommand{cmdIO: cmdIO{Out: &buf, UI: stub, Repo: git.Repo{Dir: dir}}}

	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, currentBranchIn(t, dir), "feat/child")
}

func TestStartPointFor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		selected string
		want     startPoint
		wantErr  bool
	}{
		"local":            {selected: "local: feature", want: startPoint{ref: "feature"}},
		"local with slash": {selected: "local: feat/login", want: startPoint{ref: "feat/login"}},
		"detached head":    {selected: "local: HEAD (detached at cf10b5f)", want: startPoint{ref: "cf10b5f"}},
		// current branch carries the HEAD marker for search; it strips back off.
		"current local":        {selected: "local: main (checked out here. HEAD)", want: startPoint{ref: "main"}},
		"current local/remote": {selected: "local/remote: origin/main (checked out here. HEAD)", want: startPoint{ref: "main"}},
		"local/remote":         {selected: "local/remote: origin/feat/login", want: startPoint{ref: "feat/login"}},
		"remote": {selected: "remote: origin/feat/login", want: startPoint{
			ref: "origin/feat/login", remote: "origin", branch: "feat/login",
		}},
		"remote without slash": {selected: "remote: mangled", wantErr: true},
		"local/remote bare":    {selected: "local/remote: mangled", wantErr: true},
		"unknown type":         {selected: "tag: v1.0.0", wantErr: true},
		"no type tag":          {selected: "feature", wantErr: true},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := startPointFor(c.selected)
			if c.wantErr {
				assert.Error(t, err, assert.AnyError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, got.ref, c.want.ref)
			assert.Equal(t, got.remote, c.want.remote)
			assert.Equal(t, got.branch, c.want.branch)
		})
	}
}
