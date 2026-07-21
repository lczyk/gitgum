package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
		assert.Equal(t, matchesRepoDir(s, "foo"), true)
	}
	for _, s := range no {
		assert.Equal(t, matchesRepoDir(s, "foo"), false)
	}
}

func TestCheckRemoteNaming(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "https://github.com/lczyk/gitgum")
	temp_repo.RunGit(t, dir, "remote", "add", "nsklikas", "https://github.com/nsklikas/gitgum")
	temp_repo.RunGit(t, dir, "remote", "add", "custom", "git@example.com:team/gitgum.git")

	findings := checkRemoteNaming(git.Repo{Dir: dir})

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

	findings := checkRemoteNaming(git.Repo{Dir: dir})
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

	findings := checkUpstreams(git.Repo{Dir: dir})
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

	findings := checkUpstreams(git.Repo{Dir: dir})
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

	findings := checkLayout(git.Repo{Dir: repo})

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

	findings := checkLayout(git.Repo{Dir: repo})

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

	findings := checkLayout(git.Repo{Dir: repo})
	assert.Equal(t, len(findings), 0)
}

func TestRenderFindings_Clean(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	renderFindings(&buf, nil)
	assert.Equal(t, buf.String(), "no issues found.\n")
}

func TestRenderFindings_Grouped(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	var buf bytes.Buffer
	renderFindings(&buf, []Finding{
		{Check: "upstream-consistency", Severity: SevWarning, Message: "two remotes"},
		{Check: "remote-naming", Severity: SevFixable, Message: "misnamed", Fix: "git remote rename a b"},
	})
	got := buf.String()
	// fixable sorts before warning regardless of input order.
	want := strings.Join([]string{
		"fixable [remote-naming] misnamed",
		"    fix: git remote rename a b",
		"warning [upstream-consistency] two remotes",
		"",
		"1 fixable, 1 warning(s).",
		"",
	}, "\n")
	assert.Equal(t, got, want)
}
