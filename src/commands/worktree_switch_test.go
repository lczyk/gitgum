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

func TestNumberWorktrees(t *testing.T) {
	t.Parallel()
	wts := []git.Worktree{
		{Path: "/w/foo", Branch: "main"},
		{Path: "/w/foo-3", Branch: "c"},
		{Path: "/w/foo-2", Branch: "b"},
		{Path: "/w/foo-02", Branch: "dup"},
		{Path: "/w/scratch", Branch: "d"},
		{Path: "/elsewhere/foo-4", Branch: "e"},
		{Path: "/w/foo-5", Prunable: true},
		{Path: "/w/foo-6", Bare: true},
	}
	got, warnings := numberWorktrees(wts, "foo")
	require.Equal(t, len(got), 3)
	assert.Equal(t, got[0], numberedWorktree{Index: 1, Path: "/w/foo"})
	assert.Equal(t, got[1], numberedWorktree{Index: 2, Path: "/w/foo-2"})
	assert.Equal(t, got[2], numberedWorktree{Index: 3, Path: "/w/foo-3"})
	require.Equal(t, len(warnings), 1)
	assert.ContainsString(t, warnings[0], "foo-02")
}

func TestPickWorktree(t *testing.T) {
	t.Parallel()
	entries := []numberedWorktree{{1, "/w/foo"}, {2, "/w/foo-2"}, {4, "/w/foo-4"}}
	cases := map[string]struct {
		current string
		n       int
		back    bool
		want    string
		wantErr string
	}{
		"by number":                          {current: "/w/foo", n: 2, want: "/w/foo-2"},
		"by number, gap in numbering":        {current: "/w/foo", n: 4, want: "/w/foo-4"},
		"missing number lists what is":       {current: "/w/foo", n: 3, wantErr: "no worktree numbered 3; have 1, 2, 4"},
		"negative number":                    {current: "/w/foo", n: -1, wantErr: "must be positive"},
		"cycle forward":                      {current: "/w/foo", want: "/w/foo-2"},
		"cycle skips the gap":                {current: "/w/foo-2", want: "/w/foo-4"},
		"cycle wraps":                        {current: "/w/foo-4", want: "/w/foo"},
		"cycle from outside lands first":     {current: "/w/scratch", want: "/w/foo"},
		"cycle back":                         {current: "/w/foo-2", back: true, want: "/w/foo"},
		"cycle back skips the gap":           {current: "/w/foo-4", back: true, want: "/w/foo-2"},
		"cycle back wraps":                   {current: "/w/foo", back: true, want: "/w/foo-4"},
		"cycle back from outside lands last": {current: "/w/scratch", back: true, want: "/w/foo-4"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			step := 1
			if tc.back {
				step = -1
			}
			got, err := pickWorktree(entries, tc.current, tc.n, step)
			if tc.wantErr != "" {
				assert.Error(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, got.Path, tc.want)
		})
	}

	_, err := pickWorktree(nil, "/w/foo", 0, 1)
	assert.Error(t, err, "no worktrees follow")
	_, err = pickWorktree(entries[:1], "/w/foo", 0, 1)
	assert.Error(t, err, "only one numbered worktree")
	_, err = pickWorktree(entries[:1], "/w/foo", 0, -1)
	assert.Error(t, err, "only one numbered worktree")
	only, err := pickWorktree(entries[:1], "/w/scratch", 0, 1)
	require.NoError(t, err)
	assert.Equal(t, only.Path, "/w/foo")
}

func TestRepoDirName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, repoDirName([]string{"lczyk https://github.com/lczyk/gitgum"}, "/w/checkout"), "gitgum")
	assert.Equal(t, repoDirName([]string{"a https://github.com/a/x", "b https://github.com/b/y"}, "/w/checkout"), "checkout")
	assert.Equal(t, repoDirName([]string{"custom git@example.com:team/x.git"}, "/w/checkout"), "checkout")
	assert.Equal(t, repoDirName(nil, "/w/checkout/"), "checkout")
}

// worktreeFixture lays out <root>/gitgum with sibling worktrees gitgum-2,
// gitgum-3 and a stray-named one, and returns the resolved paths keyed by
// basename. The forge remote makes "gitgum" the canonical dir name, so the
// main checkout's own basename is not what the numbering is derived from.
func worktreeFixture(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	main := filepath.Join(root, "gitgum")
	require.NoError(t, os.MkdirAll(main, 0o755), "mkdir")
	temp_repo.RunGit(t, main, "init", "-b", "main")
	temp_repo.RunGit(t, main, "config", "user.name", "Test User")
	temp_repo.RunGit(t, main, "config", "user.email", "test@example.com")
	temp_repo.RunGit(t, main, "config", "commit.gpgsign", "false")
	temp_repo.RunGit(t, main, "config", "core.hooksPath", ".git/hooks")
	temp_repo.CreateCommit(t, main, "README.md", "# test\n", "chore: init")
	temp_repo.RunGit(t, main, "remote", "add", "lczyk", "https://github.com/lczyk/gitgum")

	names := []string{"gitgum-2", "gitgum-3", "scratch"}
	for _, name := range names {
		temp_repo.RunGit(t, main, "worktree", "add", filepath.Join(root, name), "-b", name)
	}
	paths := map[string]string{"gitgum": canonicalPath(main)}
	for _, name := range names {
		paths[name] = canonicalPath(filepath.Join(root, name))
	}
	return paths
}

// headRowOf is the `gg status head` row of a fixture worktree, which sits on a
// branch named after its directory (the main one on main), all at one commit.
func headRowOf(t *testing.T, paths map[string]string, name string) string {
	t.Helper()
	branch := name
	if name == "gitgum" {
		branch = "main"
	}
	sha := strings.TrimSpace(temp_repo.RunGit(t, paths[name], "rev-parse", "--short", "HEAD"))
	return "* " + branch + " " + sha + "\n"
}

func TestWorktreeSwitch_Execute(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	paths := worktreeFixture(t)

	run := func(from string, n int) (string, string, error) {
		var out, errOut strings.Builder
		cmd := &WorktreeSwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errOut, Repo: git.Repo{Dir: paths[from]}}}
		cmd.Args.N = n
		err := cmd.Execute(nil)
		return strings.TrimSpace(out.String()), errOut.String(), err
	}

	cases := map[string]struct {
		from string
		n    int
		want string
	}{
		"cycle from main":       {from: "gitgum", want: "gitgum-2"},
		"cycle from middle":     {from: "gitgum-2", want: "gitgum-3"},
		"cycle wraps to main":   {from: "gitgum-3", want: "gitgum"},
		"cycle from stray":      {from: "scratch", want: "gitgum"},
		"number from anywhere":  {from: "scratch", n: 3, want: "gitgum-3"},
		"number one is main":    {from: "gitgum-2", n: 1, want: "gitgum"},
		"number from subdir ok": {from: "gitgum", n: 2, want: "gitgum-2"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, announced, err := run(tc.from, tc.n)
			require.NoError(t, err)
			assert.Equal(t, got, paths[tc.want])
			assert.Equal(t, announced, headRowOf(t, paths, tc.from)+headRowOf(t, paths, tc.want))
		})
	}

	_, announced, err := run("gitgum", 9)
	assert.Error(t, err, "no worktree numbered 9; have 1, 2, 3")
	assert.Equal(t, announced, "")
}

func TestWorktreeSwitchBack_Execute(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	paths := worktreeFixture(t)

	cases := map[string]struct {
		from string
		want string
	}{
		"back from middle":   {from: "gitgum-2", want: "gitgum"},
		"back wraps to last": {from: "gitgum", want: "gitgum-3"},
		"back from stray":    {from: "scratch", want: "gitgum-3"},
		"back from last":     {from: "gitgum-3", want: "gitgum-2"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out, errOut strings.Builder
			cmd := &WorktreeSwitchBackCommand{cmdIO: cmdIO{Out: &out, Err: &errOut, Repo: git.Repo{Dir: paths[tc.from]}}}
			require.NoError(t, cmd.Execute(nil))
			assert.Equal(t, strings.TrimSpace(out.String()), paths[tc.want])
			assert.Equal(t, errOut.String(), headRowOf(t, paths, tc.from)+headRowOf(t, paths, tc.want))
		})
	}

	var out strings.Builder
	cmd := &WorktreeSwitchBackCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: paths["gitgum"]}}}
	assert.Error(t, cmd.Execute([]string{"3"}), "takes no arguments")
	assert.Equal(t, out.String(), "")
}

// A detached worktree announces itself the way `gg status head` would, which
// is a row per containing ref once there are several.
func TestWorktreeSwitch_DetachedTarget(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	paths := worktreeFixture(t)
	temp_repo.RunGit(t, paths["gitgum-2"], "checkout", "--detach")

	var out, errOut strings.Builder
	cmd := &WorktreeSwitchCommand{cmdIO: cmdIO{Out: &out, Err: &errOut, Repo: git.Repo{Dir: paths["gitgum"]}}}
	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, strings.TrimSpace(out.String()), paths["gitgum-2"])
	rows := strings.Split(strings.TrimSpace(errOut.String()), "\n")
	assert.Equal(t, rows[0]+"\n", headRowOf(t, paths, "gitgum"))
	assert.Equal(t, strings.HasPrefix(rows[1], "* HEAD "), true)
	assert.Equal(t, len(rows) > 2, true)
}

// A subdirectory of a worktree is still that worktree: the cycle is computed
// from the toplevel, not the cwd.
func TestWorktreeSwitch_FromSubdir(t *testing.T) {
	t.Parallel()
	paths := worktreeFixture(t)
	sub := filepath.Join(paths["gitgum-2"], "deep", "er")
	require.NoError(t, os.MkdirAll(sub, 0o755), "mkdir")

	var out strings.Builder
	cmd := &WorktreeSwitchCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: sub}}}
	require.NoError(t, cmd.Execute(nil))
	assert.Equal(t, strings.TrimSpace(out.String()), paths["gitgum-3"])
}

func TestWorktreeSwitch_SingleWorktree(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	var out strings.Builder
	cmd := &WorktreeSwitchCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)
	assert.Error(t, err, "only one numbered worktree")
	assert.Equal(t, out.String(), "")
}
