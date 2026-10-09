package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestOperationRows_Rebase(t *testing.T) {
	t.Parallel()
	op := git.Operation{
		Name:   git.OpRebase,
		Branch: "topic",
		Onto:   "f9ba3c7",
		Step:   1,
		Total:  5,
		Stop:   git.OpStopEditing,
		Last:   git.OpCommand{Verb: "edit", Hash: "083cdfe", Subject: "chore: two"},
		Next: []git.OpCommand{
			{Verb: "pick", Hash: "aaaaaaa", Subject: "chore: three"},
			{Verb: "pick", Hash: "bbbbbbb", Subject: "chore: four"},
			{Verb: "exec", Subject: "make test"},
			{Verb: "pick", Hash: "ccccccc", Subject: "chore: five"},
			{Verb: "pick", Hash: "ddddddd", Subject: "chore: six"},
		},
	}
	got := strings.Join(operationRows(op, false), "\n")
	assert.Equal(t, got, strings.Join([]string{
		"  rebasing topic onto f9ba3c7 (1/5, editing a commit)",
		"    done  edit 083cdfe chore: two",
		"    next  pick aaaaaaa chore: three",
		"          pick bbbbbbb chore: four",
		"          exec make test",
		"          ... 2 more",
	}, "\n"))
}

func TestOperationRows_MergeCherryPickBisect(t *testing.T) {
	t.Parallel()
	merge := strings.Join(operationRows(git.Operation{
		Name: git.OpMerge, Stop: git.OpStopConflicts, Subject: "Merge branch 'side'",
	}, false), "\n")
	assert.Equal(t, merge, "  merging (conflicts)\n    Merge branch 'side'")

	pick := strings.Join(operationRows(git.Operation{
		Name: git.OpCherryPick, Head: "68e7a1b", Stop: git.OpStopConflicts, Subject: "chore: topic",
	}, false), "\n")
	assert.Equal(t, pick, "  cherry-picking 68e7a1b (conflicts)\n    chore: topic")

	bisect := strings.Join(operationRows(git.Operation{Name: git.OpBisect, Branch: "main"}, false), "\n")
	assert.Equal(t, bisect, "  bisecting, from main")

	assert.Equal(t, len(operationRows(git.Operation{}, false)), 0)
}

func TestOperationRows_ColorStripsToPlain(t *testing.T) {
	t.Parallel()
	op := git.Operation{
		Name: git.OpRebase, Branch: "topic", Onto: "abc1234",
		Step: 1, Total: 1, Stop: git.OpStopConflicts,
		Last: git.OpCommand{Verb: "pick", Hash: "abc1234", Subject: "chore: one"},
	}
	plain := strings.Join(operationRows(op, false), "\n")
	colored := strings.Join(operationRows(op, true), "\n")
	assert.Equal(t, stripAnsi(colored), plain)
	assert.ContainsString(t, colored, ansiBoldRed+"(1/1, conflicts)"+ansiReset)
	assert.ContainsString(t, colored, ansiBoldGreen+"topic"+ansiReset)
	assert.ContainsString(t, colored, ansiYellow+"abc1234"+ansiReset)
}

func TestStatusCommand_HeadReportsRebase(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	temp_repo.WriteFile(t, dir, "README.md", "from other\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: other side")
	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.WriteFile(t, dir, "README.md", "from main\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: main side")
	out, err := temp_repo.RunGitAllowFail(t, dir, "rebase", "other")
	require.That(t, err != nil, "the rebase should have conflicted: %s", out)

	var buf strings.Builder
	cmd := &StatusCommand{cmdIO: cmdIO{Out: &buf, Repo: git.Repo{Dir: dir}}}
	require.NoError(t, cmd.Execute([]string{"head"}))
	got := stripAnsi(buf.String())
	assert.ContainsString(t, got, "rebasing main onto ")
	assert.ContainsString(t, got, "(1/1, conflicts)")
	assert.ContainsString(t, got, "done  pick ")
	assert.ContainsString(t, got, "chore: main side")
}
