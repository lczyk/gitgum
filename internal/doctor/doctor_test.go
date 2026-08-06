package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// initRepoAt initialises a git repo at an explicit path (temp_repo's helpers
// pick their own random dir names; layout checks need controlled basenames).
func initRepoAt(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755), "mkdir")
	temp_repo.RunGit(t, dir, "init", "-b", "main")
	temp_repo.RunGit(t, dir, "config", "user.name", "Test User")
	temp_repo.RunGit(t, dir, "config", "user.email", "test@example.com")
	temp_repo.RunGit(t, dir, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, dir, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, dir, "README.md", "# test\n", "chore: init")
}

func TestMatchesRepoDir(t *testing.T) {
	t.Parallel()
	yes := []string{"foo", "foo-2", "foo-3", "foo-42"}
	no := []string{"foobar", "foo-", "foo-2a", "foo2", "bar", "foo-x", "-foo"}
	for _, s := range yes {
		assert.Equal(t, MatchesRepoDir(s, "foo"), true)
	}
	for _, s := range no {
		assert.Equal(t, MatchesRepoDir(s, "foo"), false)
	}
}

func TestCheckRemoteNaming(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/lczyk/gitgum")
	temp_repo.RunGit(t, dir, "remote", "add", "nsklikas", "https://github.com/nsklikas/gitgum")
	temp_repo.RunGit(t, dir, "remote", "add", "custom", "git@example.com:team/gitgum.git")

	findings := checkRemoteNaming(newFacts(git.Repo{Dir: dir}))

	// origin -> fixable (should be "lczyk"); custom -> warning; nsklikas -> clean.
	var fixable, warning []Finding
	for _, f := range findings {
		if f.Severity == SevFixable {
			fixable = append(fixable, f)
		} else {
			warning = append(warning, f)
		}
	}
	require.Equal(t, len(fixable), 1, "one fixable (origin)")
	assert.ContainsString(t, fixable[0].Message, `remote "origin"`)
	assert.Equal(t, fixable[0].Fix, "git remote rename origin lczyk")

	require.Equal(t, len(warning), 1, "one warning (custom url)")
	assert.ContainsString(t, warning[0].Message, `remote "custom"`)
}

func TestCheckRemoteNaming_Clean(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")

	findings := checkRemoteNaming(newFacts(git.Repo{Dir: dir}))
	assert.Equal(t, len(findings), 0)
}

func TestCheckUpstreams_Divergent(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "checkout", "-b", "feat")
	temp_repo.RunGit(t, dir, "checkout", "main")
	// two remotes must exist for %(upstream:short) to resolve.
	temp_repo.RunGit(t, dir, "remote", "add", "alice", "https://github.com/alice/x")
	temp_repo.RunGit(t, dir, "remote", "add", "bob", "https://github.com/bob/x")
	// point the two branches at different remotes via config directly.
	temp_repo.RunGit(t, dir, "config", "branch.main.remote", "alice")
	temp_repo.RunGit(t, dir, "config", "branch.main.merge", "refs/heads/main")
	temp_repo.RunGit(t, dir, "config", "branch.feat.remote", "bob")
	temp_repo.RunGit(t, dir, "config", "branch.feat.merge", "refs/heads/feat")

	findings := checkUpstreams(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1, "divergent upstreams -> one warning")
	assert.Equal(t, findings[0].Severity, SevWarning)
	assert.ContainsString(t, findings[0].Message, "alice")
	assert.ContainsString(t, findings[0].Message, "bob")
}

func TestCheckUpstreams_Consistent(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "alice", "https://github.com/alice/x")
	temp_repo.RunGit(t, dir, "config", "branch.main.remote", "alice")
	temp_repo.RunGit(t, dir, "config", "branch.main.merge", "refs/heads/main")

	findings := checkUpstreams(newFacts(git.Repo{Dir: dir}))
	assert.Equal(t, len(findings), 0)
}

func TestCheckLayout_StraySibling(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "gitgum")
	initRepoAt(t, repo)
	temp_repo.RunGit(t, repo, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")
	// a proper worktree sibling, and a stray dir that isn't one.
	temp_repo.RunGit(t, repo, "worktree", "add", filepath.Join(root, "gitgum-2"), "-b", "feat")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "gitgum-3"), 0o755), "mkdir stray")

	findings := checkLayout(newFacts(git.Repo{Dir: repo}))

	var adjacent []Finding
	for _, f := range findings {
		if f.Check == "adjacent-worktree" {
			adjacent = append(adjacent, f)
		}
	}
	require.Equal(t, len(adjacent), 1, "gitgum-3 flagged, gitgum-2 not")
	assert.ContainsString(t, adjacent[0].Message, "gitgum-3")
}

func TestCheckLayout_DirNaming(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "wrongname")
	initRepoAt(t, repo)
	temp_repo.RunGit(t, repo, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")

	findings := checkLayout(newFacts(git.Repo{Dir: repo}))

	var naming []Finding
	for _, f := range findings {
		if f.Check == "dir-naming" {
			naming = append(naming, f)
		}
	}
	require.Equal(t, len(naming), 1, "dir named wrongname flagged")
	assert.Equal(t, naming[0].Severity, SevFixable)
	assert.ContainsString(t, naming[0].Fix, "gitgum")
}

func TestCheckLayout_CleanNoGithubRemote(t *testing.T) {
	t.Parallel()
	// no github remote -> can't derive repo name -> no dir/adjacent findings.
	root := t.TempDir()
	repo := filepath.Join(root, "whatever")
	initRepoAt(t, repo)

	findings := checkLayout(newFacts(git.Repo{Dir: repo}))
	assert.Equal(t, len(findings), 0)
}

func TestCheckPRBranchNaming_SingleRemoteFixable(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/lczyk/gitgum")
	temp_repo.RunGit(t, dir, "branch", "pr-51")

	findings := checkPRBranchNaming(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1, "one old pr-N branch")
	assert.Equal(t, findings[0].Check, "pr-branch-naming")
	assert.Equal(t, findings[0].Severity, SevFixable)
	assert.Equal(t, findings[0].Fix, "git branch -m pr-51 pr/origin/51")
}

func TestCheckPRBranchNaming_AmbiguousRemoteWarns(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "a", "https://github.com/a/x")
	temp_repo.RunGit(t, dir, "remote", "add", "b", "https://github.com/b/x")
	temp_repo.RunGit(t, dir, "branch", "pr-7")

	findings := checkPRBranchNaming(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1, "one old pr-N branch")
	assert.Equal(t, findings[0].Severity, SevWarning)
	assert.Equal(t, findings[0].Fix, "")
}

func TestCheckPRBranchNaming_NewSchemeIgnored(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/lczyk/gitgum")
	temp_repo.RunGit(t, dir, "branch", "pr/origin/9")
	temp_repo.RunGit(t, dir, "branch", "prancing") // not a PR branch

	assert.Equal(t, len(checkPRBranchNaming(newFacts(git.Repo{Dir: dir}))), 0)
}

func TestCheckPrunableWorktrees(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "gitgum")
	initRepoAt(t, repo)
	wtPath := filepath.Join(root, "gitgum-2")
	temp_repo.RunGit(t, repo, "worktree", "add", wtPath, "-b", "feat")
	require.NoError(t, os.RemoveAll(wtPath), "remove worktree dir out from under git")

	findings := checkPrunableWorktrees(newFacts(git.Repo{Dir: repo}))
	require.Equal(t, len(findings), 1, "the removed worktree is prunable")
	assert.Equal(t, findings[0].Check, "prunable-worktree")
	assert.Equal(t, findings[0].Severity, SevFixable)
	assert.Equal(t, findings[0].Fix, "git worktree prune")
}

func TestCheckGoneUpstreams(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/x/y")
	// upstream configured, but refs/remotes/origin/main was never fetched -> gone.
	temp_repo.RunGit(t, dir, "config", "branch.main.remote", "origin")
	temp_repo.RunGit(t, dir, "config", "branch.main.merge", "refs/heads/main")

	findings := checkGoneUpstreams(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1, "main's upstream is gone")
	assert.Equal(t, findings[0].Check, "gone-upstream")
	assert.Equal(t, findings[0].Severity, SevWarning)
	assert.ContainsString(t, findings[0].Message, "main")
}

func TestCheckDuplicateRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/lczyk/gitgum")
	temp_repo.RunGit(t, dir, "remote", "add", "dup", "https://github.com/lczyk/gitgum")

	findings := checkDuplicateRemotes(newFacts(git.Repo{Dir: dir}))
	require.Equal(t, len(findings), 1, "origin and dup share a url")
	assert.Equal(t, findings[0].Severity, SevWarning)
	assert.ContainsString(t, findings[0].Message, "dup")
	assert.ContainsString(t, findings[0].Message, "origin")
}

// Diagnose aggregates findings from every check. A repo in a correctly-named
// dir with a correctly-named remote is clean.
func TestDiagnose_CleanRepo(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(t.TempDir(), "gitgum")
	initRepoAt(t, repo)
	temp_repo.RunGit(t, repo, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")
	assert.Equal(t, len(Diagnose(git.Repo{Dir: repo})), 0)
}

func TestDiagnose_MisnamedRemote(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(t.TempDir(), "gitgum")
	initRepoAt(t, repo)
	temp_repo.RunGit(t, repo, "remote", "add", "origin", "https://github.com/lczyk/gitgum")

	findings := Diagnose(git.Repo{Dir: repo})
	require.Equal(t, len(findings), 1, "only remote-naming fires (dir + repo are named gitgum)")
	assert.Equal(t, findings[0].Check, "remote-naming")
	assert.Equal(t, findings[0].Severity, SevFixable)
}
