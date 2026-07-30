package commands

import (
	"os"
	"path/filepath"
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
			p := buildClonePlan(parsed.Ref, parsed.Route, tc.raw, tc.dir, tc.depth)
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

func TestCheckExistingDest(t *testing.T) {
	t.Parallel()

	// check drives clone's dest inspection for want ("user/repo" spelling or a
	// pinned url) against dir, returning the outcome and captured stdout.
	check := func(t *testing.T, want, dir string) (done bool, err error, out string) {
		t.Helper()
		var buf strings.Builder
		c := &CloneCommand{cmdIO: cmdIO{Out: &buf, Err: &buf}}
		c.Args.Dir = dir
		done, err = c.checkExistingDest(mustRef(t, want))
		return done, err, buf.String()
	}

	t.Run("absent dir falls through to git", func(t *testing.T) {
		t.Parallel()
		done, err, _ := check(t, "canonical/rockcraft", filepath.Join(t.TempDir(), "rockcraft"))
		require.NoError(t, err, "check")
		assert.That(t, !done, "done")
	})

	t.Run("empty dir falls through to git", func(t *testing.T) {
		t.Parallel()
		done, err, _ := check(t, "canonical/rockcraft", t.TempDir())
		require.NoError(t, err, "check")
		assert.That(t, !done, "done")
	})

	t.Run("non-repo dir errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "junk"), []byte("x"), 0o644), "write")
		done, err, _ := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
		assert.ContainsString(t, err.Error(), "not a git repository")
	})

	t.Run("matching remote reports already cloned", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/rockcraft")
		done, err, out := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		require.NoError(t, err, "check")
		assert.ContainsString(t, out, "already cloned")
	})

	t.Run("other slug errors and names it", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "foo", "https://github.com/foo/rockcraft")
		done, err, _ := check(t, "canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
		assert.ContainsString(t, err.Error(), "clone of github.com/foo/rockcraft")
	})

	t.Run("host-pinned request needs the same host", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/rockcraft")
		done, err, _ := check(t, "gitlab.com/canonical/rockcraft", dir)
		assert.That(t, done, "done")
		assert.Error(t, err, assert.AnyError, "check")
	})

	t.Run("repo without remotes errors", func(t *testing.T) {
		t.Parallel()
		dir := temp_repo.NewRepo(t)
		done, err, _ := check(t, "canonical/rockcraft", dir)
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
			cmdIO: cmdIO{UI: sel},
			probe: func(u string) bool { return u == "https://gitlab.com/canonical/cbs-tools" },
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
			cmdIO: cmdIO{UI: sel},
			probe: func(u string) bool { return true }, // exists everywhere
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
		c := &CloneCommand{probe: func(u string) bool { return false }}
		_, err := c.resolveShorthand(ref)
		require.Error(t, err, "not found")
	})
}
