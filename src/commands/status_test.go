package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestStatusCommand_NotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cmd := &StatusCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "should error when not in git repo")
	assert.ContainsString(t, err.Error(), "not inside a git repository")
}

// Bare `gg status` is "changes,head": no branches, no remotes, and on a clean
// repo the CHANGES section suppresses itself, leaving HEAD alone -- which,
// being the sole surviving section, prints without a header.
func TestStatusCommand_InGitRepo(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	require.NoError(t, err, "should succeed in git repo")
	output := buf.String()
	assert.ContainsString(t, output, "* main")
	for _, absent := range []string{"BRANCHES", "REMOTES", "CHANGES"} {
		if strings.Contains(output, absent) {
			t.Errorf("clean repo default status should not emit %s section, got:\n%s", absent, output)
		}
	}
}

func TestStatusCommand_WithChanges(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	// untracked file triggers git status --short --branch to emit a change line
	temp_repo.WriteFile(t, dir, "untracked.txt", "hello\n")

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute(nil)

	require.NoError(t, err, "should succeed with pending changes")
	output := buf.String()
	assert.ContainsString(t, output, "CHANGES")
	assert.ContainsString(t, output, "HEAD")
}

func TestStatusCommand_SectionSelection(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cases := map[string]struct {
		args    []string
		present []string
		absent  []string
	}{
		"single section renders bare": {
			args:   []string{"branch"},
			absent: []string{"BRANCHES", "REMOTES", "HEAD"},
		},
		"short names": {
			args:    []string{"b,h"},
			present: []string{"BRANCHES", "HEAD"},
			absent:  []string{"REMOTES"},
		},
		"spaces are stripped": {
			args:    []string{" b ,   head   "},
			present: []string{"BRANCHES", "HEAD"},
		},
		"worktree section": {
			args:    []string{"worktree,head"},
			present: []string{"WORKTREES", "HEAD"},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf strings.Builder
			cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
			require.NoError(t, cmd.Execute(tt.args))

			output := buf.String()
			for _, want := range tt.present {
				assert.ContainsString(t, output, want)
			}
			for _, absent := range tt.absent {
				if strings.Contains(output, absent) {
					t.Errorf("%v should not emit %s, got:\n%s", tt.args, absent, output)
				}
			}
		})
	}
}

func TestStatusCommand_UnknownSection(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &StatusCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Repo: git.Repo{Dir: dir}}}
	err := cmd.Execute([]string{"branch,bogus"})

	assert.Error(t, err, assert.AnyError, "unknown section should error")
	assert.ContainsString(t, err.Error(), `unknown status section "bogus"`)
}

func TestParseSections(t *testing.T) {
	t.Parallel()
	ids := func(secs []statusSection) []string {
		out := make([]string, len(secs))
		for i, sec := range secs {
			out[i] = sec.id
		}
		return out
	}

	cases := map[string]struct {
		spec     string
		expected []string
	}{
		"default":                     {spec: "changes,head", expected: []string{"changes", "head"}},
		"short names":                 {spec: "b,r,w", expected: []string{"branch", "remote", "worktree"}},
		"case insensitive":            {spec: "Branch,HEAD", expected: []string{"branch", "head"}},
		"whitespace stripped":         {spec: "  b ,   head   ", expected: []string{"branch", "head"}},
		"order preserved":             {spec: "head,branch", expected: []string{"head", "branch"}},
		"duplicates keep last":        {spec: "b,r,b,w", expected: []string{"remote", "branch", "worktree"}},
		"long and short are the same": {spec: "branch,b", expected: []string{"branch"}},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			secs, err := parseSections(tt.spec)
			require.NoError(t, err)
			assert.EqualArrays(t, ids(secs), tt.expected)
		})
	}
}

func TestParseSections_Errors(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{"", "   ", ",,", "nope", "branches"} {
		_, err := parseSections(spec)
		assert.Error(t, err, assert.AnyError, "spec %q should error", spec)
	}
}

func TestStatusCommand_FollowRequiresTTY(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	interval := 2.0
	cmd := &StatusCommand{
		cmdIO:  cmdIO{Out: &strings.Builder{}, Repo: git.Repo{Dir: dir}},
		Follow: &interval,
	}
	err := cmd.Execute(nil)

	assert.Error(t, err, assert.AnyError, "follow without tty should error")
	assert.ContainsString(t, err.Error(), "tty")
}
