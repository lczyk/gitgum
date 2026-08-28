package commands

import (
	"reflect"
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
		// no remote is configured in a temp repo, and REMOTES suppresses
		// itself when there is nothing to list.
		"all section": {
			args:    []string{"all"},
			present: []string{"BRANCHES", "WORKTREES", "HEAD"},
		},
		"all short name": {
			args:    []string{"a"},
			present: []string{"BRANCHES", "WORKTREES", "HEAD"},
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
		"all expands in order":        {spec: "all", expected: []string{"branch", "remote", "worktree", "changes", "head"}},
		"all short name":              {spec: "a", expected: []string{"branch", "remote", "worktree", "changes", "head"}},
		"all is case insensitive":     {spec: "ALL", expected: []string{"branch", "remote", "worktree", "changes", "head"}},
		"a section after all moves":   {spec: "all,branch", expected: []string{"remote", "worktree", "changes", "head", "branch"}},
		"all after a section wins":    {spec: "branch,all", expected: []string{"branch", "remote", "worktree", "changes", "head"}},
		"all twice is once":           {spec: "all,a", expected: []string{"branch", "remote", "worktree", "changes", "head"}},
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

// The SECTIONS help text is what `gg status -h` answers "which sections are
// there?" with, so it has to name the same ones parseSections accepts -- and
// the tags that put it under `-h` in the first place have to still be there.
func TestStatusCommand_SectionsHelpNamesEverySection(t *testing.T) {
	t.Parallel()
	args, ok := reflect.TypeOf(StatusCommand{}).FieldByName("Args")
	require.That(t, ok, "StatusCommand should have an Args field")
	assert.Equal(t, args.Tag.Get("positional-args"), "yes")
	sections, ok := args.Type.FieldByName("Sections")
	require.That(t, ok, "Args should have a Sections field")
	assert.Equal(t, sections.Tag.Get("positional-arg-name"), "SECTIONS")
	assert.ContainsString(t, sections.Tag.Get("description"), knownSections())
}

func TestStatusCommand_AllFlag(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}, All: true}
	require.NoError(t, cmd.Execute(nil))

	output := buf.String()
	for _, want := range []string{"BRANCHES", "WORKTREES", "HEAD"} {
		assert.ContainsString(t, output, want)
	}
}

func TestStatusCommand_AllFlagRejectsASectionList(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &StatusCommand{cmdIO: cmdIO{Out: &strings.Builder{}, Repo: git.Repo{Dir: dir}}, All: true}
	err := cmd.Execute([]string{"branch"})

	assert.Error(t, err, assert.AnyError, "--all alongside a section list should error")
	assert.ContainsString(t, err.Error(), "mutually exclusive")
}

// go-flags fills Args.Sections rather than passing the section list on to
// Execute, so the two have to reach parseSections by the same route.
func TestStatusCommand_PositionalArgField(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	newCmd := func(out *strings.Builder, sections string) *StatusCommand {
		cmd := &StatusCommand{cmdIO: cmdIO{Out: out, Repo: git.Repo{Dir: dir}}}
		cmd.Args.Sections = &sections
		return cmd
	}

	var buf strings.Builder
	require.NoError(t, newCmd(&buf, "branch,remote").Execute(nil))
	assert.ContainsString(t, buf.String(), "BRANCHES")

	err := newCmd(&strings.Builder{}, "branch").Execute([]string{"head"})
	assert.Error(t, err, assert.AnyError, "a second section list should error")
	assert.ContainsString(t, err.Error(), "at most one section list")

	// an empty positional is an argument, and a rejected one: `gg status ""`
	// must not quietly fall back to the default sections.
	err = newCmd(&strings.Builder{}, "").Execute(nil)
	assert.Error(t, err, assert.AnyError, "an empty section list should error")
	assert.ContainsString(t, err.Error(), "no sections given")

	all := newCmd(&strings.Builder{}, "branch")
	all.All = true
	err = all.Execute(nil)
	assert.Error(t, err, assert.AnyError, "--all with a positional section list should error")
	assert.ContainsString(t, err.Error(), "mutually exclusive")

	allEmpty := newCmd(&strings.Builder{}, "")
	allEmpty.All = true
	err = allEmpty.Execute(nil)
	assert.Error(t, err, assert.AnyError, "--all with an empty positional should still error")
	assert.ContainsString(t, err.Error(), "mutually exclusive")
}

// The sha the HEAD section prints has to survive the whole path -- the ref
// listing, headHash, formatHeadLine -- not just formatHeadLine's own tests.
func TestStatusCommand_HeadSectionPrintsTheSha(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	short, _, err := (git.Repo{Dir: dir}).Run("rev-parse", "--short", "HEAD")
	require.NoError(t, err)
	short = strings.TrimSpace(short)
	require.That(t, short != "", "temp repo should have a commit")

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"head"}))
	assert.Equal(t, strings.TrimSpace(stripAnsi(buf.String())), "* main "+short)
}

func TestHeadHash(t *testing.T) {
	t.Parallel()
	locals := []git.LocalBranch{
		{Name: "other", Hash: "aaaaaaa"},
		{Name: "main", Hash: "1a2b3c4", Head: true},
	}
	assert.Equal(t, headHash(locals), "1a2b3c4")
	assert.Equal(t, headHash(locals[:1]), "")
	assert.Equal(t, headHash(nil), "")
}
