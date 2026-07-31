package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// mustParse parses raw or fails the test; keeps the table cases terse.
func mustParse(t *testing.T, raw string) git.ForgeURL {
	t.Helper()
	parsed, ok := git.ParseForgeURL(raw)
	require.That(t, ok, "ParseForgeURL(%q)", raw)
	return parsed
}

func mustRef(t *testing.T, raw string) git.RepoRef {
	t.Helper()
	return mustParse(t, raw).Ref
}

func TestBuildClonePlan(t *testing.T) {
	t.Parallel() // stdout isn't a tty under `go test`, so notes carry no ansi.

	tests := []struct {
		name       string
		raw        string // the spelling the user typed
		branch     string // branch the pre-flight resolved, if any
		dir        string
		depth      int
		wantArgs   []string
		wantRemote string
		wantDir    string
		wantNote   string // substring; "" means no note expected
	}{
		{
			name:       "host shorthand reconstructs canonical https + names remote",
			raw:        "github.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "www is normalised away",
			raw:        "www.github.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "full https url is cloned verbatim",
			raw:        "https://gitlab.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://gitlab.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "ssh url is not downgraded to https",
			raw:        "git@github.com:canonical/cbs-tools.git",
			wantArgs:   []string{"clone", "-o", "canonical", "git@github.com:canonical/cbs-tools.git", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			// --no-single-branch is deliberate: without it a shallow clone's
			// fetch refspec covers one branch and every other branch's upstream
			// becomes unresolvable.
			name:       "codeberg + depth",
			raw:        "codeberg.org/lczyk/gitgum",
			depth:      1,
			wantArgs:   []string{"clone", "-o", "lczyk", "--depth", "1", "--no-single-branch", "https://codeberg.org/lczyk/gitgum", "gitgum"},
			wantRemote: "lczyk",
			wantDir:    "gitgum",
		},
		{
			name:       "conforming explicit dir stays quiet",
			raw:        "github.com/canonical/cbs-tools",
			dir:        "cbs-tools-2",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools-2"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools-2",
		},
		{
			// a pasted page url addresses a page, not a repo, so it is
			// reconstructed rather than handed to git verbatim.
			name:       "pr url clones the repo it belongs to",
			raw:        "https://github.com/ml-explore/mlx/pull/3161",
			wantArgs:   []string{"clone", "-o", "ml-explore", "https://github.com/ml-explore/mlx", "mlx"},
			wantRemote: "ml-explore",
			wantDir:    "mlx",
		},
		{
			name:       "gitlab subgroup keeps its full path and project dir",
			raw:        "https://gitlab.com/group/subgroup/project",
			wantArgs:   []string{"clone", "-o", "group", "https://gitlab.com/group/subgroup/project", "project"},
			wantRemote: "group",
			wantDir:    "project",
		},
		{
			name:       "gitlab mr url strips at the /-/ sentinel",
			raw:        "https://gitlab.com/group/subgroup/project/-/merge_requests/12",
			wantArgs:   []string{"clone", "-o", "group", "https://gitlab.com/group/subgroup/project", "project"},
			wantRemote: "group",
			wantDir:    "project",
		},
		{
			name:       "non-conforming explicit dir warns",
			raw:        "github.com/canonical/cbs-tools",
			dir:        "wrongname",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "wrongname"},
			wantRemote: "canonical",
			wantDir:    "wrongname",
			wantNote:   "does not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := mustParse(t, tc.raw)
			p := buildClonePlan(parsed.Ref, parsed.Route, tc.branch, tc.raw, tc.dir, tc.depth)
			assert.EqualArrays(t, p.args, tc.wantArgs)
			assert.Equal(t, p.remote, tc.wantRemote)
			assert.Equal(t, p.dir, tc.wantDir)
			if tc.wantNote == "" {
				assert.Equal(t, p.note, "")
			} else {
				assert.ContainsString(t, p.note, tc.wantNote)
			}
		})
	}
}

func TestPlainClonePlan(t *testing.T) {
	t.Parallel()
	p := plainClonePlan("git@bitbucket.org:foo/bar.git", "", 0, "note")
	assert.EqualArrays(t, p.args, []string{"clone", "git@bitbucket.org:foo/bar.git"})
	assert.Equal(t, p.remote, "")

	p = plainClonePlan("../local", "dest", 2, "")
	assert.EqualArrays(t, p.args, []string{"clone", "--depth", "2", "--no-single-branch", "../local", "dest"})
	assert.Equal(t, p.dir, "dest")
}

func TestLongestBranchMatch(t *testing.T) {
	t.Parallel()
	heads := []string{"main", "feat", "feat/x", "release/1.2"}
	cases := map[string]string{
		"main":              "main",
		"feat":              "feat",
		"feat/x":            "feat/x",      // exact beats the shorter "feat"
		"feat/x/a/b.go":     "feat/x",      // a path under the branch
		"feat/other/a.go":   "feat",        // only the shorter branch exists
		"release/1.2/a.txt": "release/1.2", // dots and digits are unremarkable
	}
	for tail, want := range cases {
		got, ok := longestBranchMatch(heads, tail)
		if !ok || got != want {
			t.Errorf("longestBranchMatch(%q) = (%q, %v), want (%q, true)", tail, got, ok, want)
		}
	}
	if _, ok := longestBranchMatch(heads, "nope/x"); ok {
		t.Errorf("longestBranchMatch(%q) matched, want no match", "nope/x")
	}
}

func TestClonePreflight(t *testing.T) {
	t.Parallel()

	const refs = "aaa\trefs/heads/main\n" +
		"bbb\trefs/heads/feat/x\n" +
		"ccc\trefs/pull/42/head\n" +
		"ddd\trefs/pull/9/merge\n"

	// preflight resolves route against a canned ref listing, so no network.
	preflight := func(t *testing.T, rawURL string) (routePlan, error) {
		t.Helper()
		parsed := mustParse(t, rawURL)
		c := &CloneCommand{remotes: stubNetwork{lsRemote: func(string) (string, error) { return refs, nil }}}
		return c.preflight(parsed.Ref, parsed.Route)
	}

	t.Run("existing PR resolves with its advertised type", func(t *testing.T) {
		t.Parallel()
		got, err := preflight(t, "https://github.com/o/r/pull/42")
		require.NoError(t, err, "preflight")
		assert.Equal(t, got.pr.Number, 42)
		assert.Equal(t, got.pr.Type, "head")
	})

	t.Run("merge-only PR keeps that type", func(t *testing.T) {
		t.Parallel()
		got, err := preflight(t, "https://github.com/o/r/pull/9")
		require.NoError(t, err, "preflight")
		assert.Equal(t, got.pr.Type, "merge")
	})

	t.Run("missing PR fails before any transfer", func(t *testing.T) {
		t.Parallel()
		_, err := preflight(t, "https://github.com/o/r/pull/999")
		assert.Error(t, err, assert.AnyError, "preflight")
		assert.ContainsString(t, err.Error(), "no pull request #999")
	})

	t.Run("slashed branch resolves against real refs", func(t *testing.T) {
		t.Parallel()
		got, err := preflight(t, "https://github.com/o/r/tree/feat/x")
		require.NoError(t, err, "preflight")
		assert.Equal(t, got.branch, "feat/x")
	})

	t.Run("blob url drops the file path", func(t *testing.T) {
		t.Parallel()
		got, err := preflight(t, "https://github.com/o/r/blob/feat/x/a/b.go")
		require.NoError(t, err, "preflight")
		assert.Equal(t, got.branch, "feat/x")
	})

	t.Run("unknown branch fails before any transfer", func(t *testing.T) {
		t.Parallel()
		_, err := preflight(t, "https://github.com/o/r/tree/nope")
		assert.Error(t, err, assert.AnyError, "preflight")
		assert.ContainsString(t, err.Error(), "no branch matching")
	})

	t.Run("commit needs no listing", func(t *testing.T) {
		t.Parallel()
		parsed := mustParse(t, "https://github.com/o/r/commit/abc1234")
		c := &CloneCommand{remotes: stubNetwork{lsRemote: func(string) (string, error) {
			t.Error("commit route should not list refs")
			return "", nil
		}}}
		got, err := c.preflight(parsed.Ref, parsed.Route)
		require.NoError(t, err, "preflight")
		assert.Equal(t, got.commit, "abc1234")
	})

	t.Run("a route gg does not model is not acted on", func(t *testing.T) {
		t.Parallel()
		got, err := preflight(t, "https://github.com/o/r/issues/5")
		require.NoError(t, err, "preflight")
		assert.Equal(t, got, routePlan{})
	})
}

// A commit url is served by fetching that one commit into the shallow clone,
// so --depth and a commit url now compose rather than conflicting.
func TestCloneCommitURLIntoShallowClone(t *testing.T) {
	// not parallel: clones into the process working directory
	origin := temp_repo.NewRepo(t)
	for i := range 6 {
		temp_repo.CreateCommit(t, origin, fmt.Sprintf("f%d", i), "x", fmt.Sprintf("chore: c%d", i))
	}
	old := strings.TrimSpace(temp_repo.RunGit(t, origin, "rev-parse", "HEAD~5"))

	dest := filepath.Join(t.TempDir(), "shallow")
	c := &CloneCommand{Depth: 1, cmdIO: cmdIO{UI: &stubSelector{confirmAnswers: []bool{false}}}}
	plan := clonePlan{dir: dest, remote: "origin"}
	require.NoError(t, c.repo().RunWriteStream(
		"clone", "--depth", "1", "--no-single-branch", "-o", "origin", "file://"+origin, dest), "clone")

	cloned := git.Repo{Dir: dest}
	require.That(t, cloned.IsShallow(), "clone should be shallow")
	require.That(t, !cloned.HasObject(old), "old commit should start outside the boundary")

	require.NoError(t, c.checkoutCommit(cloned, plan, old), "checkoutCommit")

	assert.That(t, cloned.HasObject(old), "the commit should have been fetched on its own")
	head := strings.TrimSpace(temp_repo.RunGit(t, dest, "rev-parse", "HEAD"))
	assert.Equal(t, head, old)
	// declining the deepen leaves it shallow -- reaching the commit and having
	// history around it are separate
	assert.That(t, cloned.IsShallow(), "declining the offer should leave the clone shallow")
}

// Accepting the offer deepens to the commit's own date, which is the only
// measure available -- how far back it sits cannot be known without fetching
// the very history being asked about.
func TestCloneCommitURLAcceptsDeepen(t *testing.T) {
	origin := temp_repo.NewRepo(t)
	for i := range 6 {
		temp_repo.CreateCommit(t, origin, fmt.Sprintf("f%d", i), "x", fmt.Sprintf("chore: c%d", i))
	}
	old := strings.TrimSpace(temp_repo.RunGit(t, origin, "rev-parse", "HEAD~5"))

	dest := filepath.Join(t.TempDir(), "shallow")
	stub := &stubSelector{confirmAnswers: []bool{true}}
	c := &CloneCommand{Depth: 1, cmdIO: cmdIO{UI: stub}}
	plan := clonePlan{dir: dest, remote: "origin"}
	require.NoError(t, c.repo().RunWriteStream(
		"clone", "--depth", "1", "--no-single-branch", "-o", "origin", "file://"+origin, dest), "clone")

	cloned := git.Repo{Dir: dest}
	countBranch := func() int {
		out := strings.TrimSpace(temp_repo.RunGit(t, dest, "rev-list", "--count", "origin/main"))
		n, err := strconv.Atoi(out)
		require.NoError(t, err, "count")
		return n
	}
	before := countBranch()
	require.Equal(t, before, 1) // --depth 1

	require.NoError(t, c.checkoutCommit(cloned, plan, old), "checkoutCommit")

	require.Equal(t, len(stub.confirmCalls), 1)
	assert.ContainsString(t, stub.confirmCalls[0].Prompt, "Deepen")
	// the commit is now connected to the tip, which is what makes log readable
	assert.That(t, countBranch() > before,
		"deepening should have brought in the history between the commit and the tip")
}

// A server that will not serve an unadvertised object leaves a usable shallow
// clone on the default branch, with a warning rather than a failure.
func TestCloneCommitURLServerRefuses(t *testing.T) {
	origin := temp_repo.NewRepo(t)
	for i := range 6 {
		temp_repo.CreateCommit(t, origin, fmt.Sprintf("f%d", i), "x", fmt.Sprintf("chore: c%d", i))
	}
	old := strings.TrimSpace(temp_repo.RunGit(t, origin, "rev-parse", "HEAD~5"))

	dest := filepath.Join(t.TempDir(), "shallow")
	var errBuf strings.Builder
	c := &CloneCommand{Depth: 1, cmdIO: cmdIO{Err: &errBuf}}
	// point the remote at nothing, so the object fetch cannot succeed
	plan := clonePlan{dir: dest, remote: "nosuchremote"}
	require.NoError(t, c.repo().RunWriteStream(
		"clone", "--depth", "1", "--no-single-branch", "-o", "origin", "file://"+origin, dest), "clone")

	cloned := git.Repo{Dir: dest}
	before := strings.TrimSpace(temp_repo.RunGit(t, dest, "rev-parse", "--abbrev-ref", "HEAD"))

	require.NoError(t, c.checkoutCommit(cloned, plan, old), "a refused object is not a clone failure")

	assert.ContainsString(t, errBuf.String(), "outside this shallow clone")
	after := strings.TrimSpace(temp_repo.RunGit(t, dest, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, after, before) // still on the default branch, nothing detached
}

func TestCheckExistingDest(t *testing.T) {
	t.Parallel()

	// check drives clone's dest inspection for want ("user/repo" spelling or a
	// pinned url) against dir, returning the outcome and captured stdout.
	check := func(t *testing.T, want, dir string) (done bool, out string, err error) {
		t.Helper()
		var buf strings.Builder
		c := &CloneCommand{cmdIO: cmdIO{Out: &buf, Err: &buf}}
		c.Args.Dir = dir
		done, err = c.checkExistingDest(mustRef(t, want), git.Route{})
		return done, buf.String(), err
	}

	t.Run("absent dir falls through to git", func(t *testing.T) {
		t.Parallel()
		done, _, err := check(t, "canonical/rockcraft", filepath.Join(t.TempDir(), "rockcraft"))
		require.NoError(t, err, "check")
		assert.That(t, !done, "done")
	})

	t.Run("empty dir falls through to git", func(t *testing.T) {
		t.Parallel()
		done, _, err := check(t, "canonical/rockcraft", t.TempDir())
		require.NoError(t, err, "check")
		assert.That(t, !done, "done")
	})

	t.Run("non-repo dir errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "junk"), []byte("x"), 0o644), "write")
		done, _, err := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
		assert.ContainsString(t, err.Error(), "not a git repository")
	})

	t.Run("matching remote reports already cloned", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/rockcraft")
		done, out, err := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		require.NoError(t, err, "check")
		assert.ContainsString(t, out, "already cloned")
	})

	t.Run("other slug errors and names it", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "foo", "https://github.com/foo/rockcraft")
		done, _, err := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
		assert.ContainsString(t, err.Error(), "clone of github.com/foo/rockcraft")
	})

	t.Run("host-pinned request needs the same host", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/rockcraft")
		done, _, err := check(t, "gitlab.com/canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
	})

	t.Run("repo without remotes errors", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		done, _, err := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
		assert.ContainsString(t, err.Error(), "none of its remotes")
	})
}

func TestRemoteMatches(t *testing.T) {
	t.Parallel()
	gh := mustRef(t, "https://github.com/canonical/rockcraft")
	tests := []struct {
		name   string
		remote git.RepoRef
		want   git.RepoRef
		match  bool
	}{
		{"shorthand matches any forge", gh, mustRef(t, "canonical/rockcraft"), true},
		{"shorthand rejects other slug", mustRef(t, "https://github.com/foo/rockcraft"), mustRef(t, "canonical/rockcraft"), false},
		{"shorthand rejects unmodelled host", mustRef(t, "https://git.example.com/canonical/rockcraft"), mustRef(t, "canonical/rockcraft"), false},
		{"pinned host matches same host", gh, mustRef(t, "github.com/canonical/rockcraft"), true},
		{"pinned host rejects other host", gh, mustRef(t, "gitlab.com/canonical/rockcraft"), false},
		{"ssh remote matches https request", mustRef(t, "git@github.com:canonical/rockcraft.git"), mustRef(t, "https://github.com/canonical/rockcraft"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, remoteMatches(tc.remote, tc.want), tc.match)
		})
	}
}

func TestResolveShorthand(t *testing.T) {
	t.Parallel()
	ref := mustRef(t, "canonical/cbs-tools")

	t.Run("single match resolves without a prompt", func(t *testing.T) {
		sel := &stubSelector{}
		c := &CloneCommand{
			cmdIO:   cmdIO{UI: sel},
			remotes: stubNetwork{reachable: func(u string) bool { return u == "https://gitlab.com/canonical/cbs-tools" }},
		}
		got, err := c.resolveShorthand(ref)
		require.NoError(t, err, "resolve")
		assert.Equal(t, got.Forge, git.ForgeGitLab)
		assert.Equal(t, got.Host, "gitlab.com")
		assert.Equal(t, len(sel.selectCalls), 0)
	})

	t.Run("multiple matches prompt a github-first picker", func(t *testing.T) {
		sel := &stubSelector{selectAnswers: []string{"https://codeberg.org/canonical/cbs-tools"}}
		c := &CloneCommand{
			cmdIO:   cmdIO{UI: sel},
			remotes: stubNetwork{reachable: func(u string) bool { return true }}, // exists everywhere
		}
		got, err := c.resolveShorthand(ref)
		require.NoError(t, err, "resolve")
		assert.Equal(t, got.Forge, git.ForgeCodeberg)
		require.Equal(t, len(sel.selectCalls), 1)
		// options are the candidate urls in preference order (github first).
		assert.EqualArrays(t, sel.selectCalls[0].Options, []string{
			"https://github.com/canonical/cbs-tools",
			"https://gitlab.com/canonical/cbs-tools",
			"https://codeberg.org/canonical/cbs-tools",
		})
	})

	t.Run("no match errors", func(t *testing.T) {
		c := &CloneCommand{remotes: stubNetwork{reachable: func(u string) bool { return false }}}
		_, err := c.resolveShorthand(ref)
		require.Error(t, err, "not found")
	})
}
