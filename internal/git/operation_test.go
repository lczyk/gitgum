package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestParseTodoLine(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		in   string
		want OpCommand
		ok   bool
	}{
		"blank":        {"", OpCommand{}, false},
		"comment":      {"# pick later", OpCommand{}, false},
		"plain pick":   {"pick abcdef1 chore: one", OpCommand{Verb: "pick", Hash: "abcdef1", Subject: "chore: one"}, true},
		"hash subject": {"pick abcdef1234567890 # chore: one", OpCommand{Verb: "pick", Hash: "abcdef1234567890", Subject: "chore: one"}, true},
		"exec":         {"exec make test", OpCommand{Verb: "exec", Subject: "make test"}, true},
		"break":        {"break", OpCommand{Verb: "break"}, true},
		"fixup flag":   {"fixup -C abcdef1 chore: one", OpCommand{Verb: "fixup", Hash: "abcdef1", Subject: "chore: one"}, true},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseTodoLine(tt.in)
			assert.Equal(t, ok, tt.ok)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestOperation_CleanRepo(t *testing.T) {
	t.Parallel()
	_, yes := (Repo{Dir: temp_repo.NewRepo(t)}).Operation()
	assert.That(t, !yes, "an ordinary checkout is not an operation")
}

func TestOperation_ConflictedRebase(t *testing.T) {
	t.Parallel()
	dir := diverged(t)
	out, err := temp_repo.RunGitAllowFail(t, dir, "rebase", "other")
	require.That(t, err != nil, "the rebase should have conflicted: %s", out)

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a stopped rebase should be reported")
	assert.Equal(t, op.Name, OpRebase)
	assert.Equal(t, op.Branch, "main")
	assert.Equal(t, op.Stop, OpStopConflicts)
	assert.Equal(t, op.Step, 1)
	assert.Equal(t, op.Total, 1)
	assert.Equal(t, op.Last.Verb, "pick")
	assert.ContainsString(t, op.Last.Subject, "main side")
	assert.That(t, op.Onto != "", "onto should be set")
	assert.Equal(t, len(op.Next), 0)
}

func TestOperation_EditingRebase(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "chore: one")
	temp_repo.CreateCommit(t, dir, "b.txt", "b\n", "chore: two")

	script := filepath.Join(dir, "edit-first.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nawk 'NR==1 && $1==\"pick\" {$1=\"edit\"} {print}' \"$1\" > \"$1.tmp\" && mv \"$1.tmp\" \"$1\"\n"), 0o755))
	cmd := exec.Command("git", "rebase", "-i", "HEAD~2")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_SEQUENCE_EDITOR="+script, "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "edit rebase should stop cleanly: %s", out)

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "an edit stop should be reported")
	assert.Equal(t, op.Name, OpRebase)
	assert.Equal(t, op.Stop, OpStopEditing)
	assert.Equal(t, op.Step, 1)
	assert.Equal(t, op.Total, 2)
	assert.Equal(t, op.Last.Verb, "edit")
	assert.ContainsString(t, op.Last.Subject, "chore: one")
	require.Equal(t, len(op.Next), 1)
	assert.Equal(t, op.Next[0].Verb, "pick")
	assert.ContainsString(t, op.Next[0].Subject, "chore: two")
}

func TestOperation_ApplyBackendRebase(t *testing.T) {
	t.Parallel()
	dir := diverged(t)
	out, err := temp_repo.RunGitAllowFail(t, dir, "-c", "rebase.backend=apply", "rebase", "other")
	require.That(t, err != nil, "the apply rebase should have conflicted: %s", out)

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "an apply-backend rebase should be reported")
	assert.Equal(t, op.Name, OpRebase)
	assert.Equal(t, op.Branch, "main")
	assert.Equal(t, op.Stop, OpStopConflicts)
	assert.Equal(t, op.Step, 1)
	assert.That(t, op.Total >= 1, "apply backend should report a total")
	assert.ContainsString(t, op.Last.Subject, "main side")
}

func TestOperation_ConflictedMerge(t *testing.T) {
	t.Parallel()
	dir := diverged(t)
	out, err := temp_repo.RunGitAllowFail(t, dir, "merge", "other")
	require.That(t, err != nil, "the merge should have conflicted: %s", out)

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a stopped merge should be reported")
	assert.Equal(t, op.Name, OpMerge)
	assert.Equal(t, op.Stop, OpStopConflicts)
	assert.ContainsString(t, op.Subject, "other")
}

func TestOperation_ResolvedMergeIsReadyToCommit(t *testing.T) {
	t.Parallel()
	dir := diverged(t)
	_, err := temp_repo.RunGitAllowFail(t, dir, "merge", "other")
	require.That(t, err != nil, "the merge should have conflicted")
	temp_repo.WriteFile(t, dir, "README.md", "resolved\n")
	temp_repo.RunGit(t, dir, "add", "README.md")

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a resolved merge is still in progress")
	assert.Equal(t, op.Name, OpMerge)
	assert.Equal(t, op.Stop, OpStopCommit)
}

func TestOperation_CherryPick(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "checkout", "-b", "topic")
	temp_repo.WriteFile(t, dir, "README.md", "from topic\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: topic")
	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.WriteFile(t, dir, "README.md", "from main\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: main")

	out, err := temp_repo.RunGitAllowFail(t, dir, "cherry-pick", "topic")
	require.That(t, err != nil, "the cherry-pick should have conflicted: %s", out)

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a stopped cherry-pick should be reported")
	assert.Equal(t, op.Name, OpCherryPick)
	assert.Equal(t, op.Stop, OpStopConflicts)
	assert.ContainsString(t, op.Subject, "chore: topic")
	assert.That(t, op.Head != "", "the picked commit should be named")
}

func TestOperation_Bisect(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "chore: one")
	temp_repo.RunGit(t, dir, "bisect", "start", "HEAD", "HEAD~1")

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a bisect should be reported")
	assert.Equal(t, op.Name, OpBisect)
	assert.Equal(t, op.Branch, "main")
	assert.Equal(t, op.Stop, "")
}

func TestOperation_SequencerTodo(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	gitDir := strings.TrimSpace(temp_repo.RunGit(t, dir, "rev-parse", "--absolute-git-dir"))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "sequencer"), 0o755))
	todo := "pick abcdef1 chore: one\npick abcdef2 chore: two\n"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "sequencer", "todo"), []byte(todo), 0o644))

	op, yes := (Repo{Dir: dir}).Operation()
	require.That(t, yes, "a sequencer todo should be reported")
	assert.Equal(t, op.Name, OpCherryPick)
	require.Equal(t, len(op.Next), 2)
	assert.Equal(t, op.Next[0].Subject, "chore: one")
	assert.Equal(t, op.Next[1].Verb, "pick")
}

// diverged is a repo whose main and other branches each committed a different
// README, so a merge or rebase of one onto the other stops on a conflict.
func diverged(t *testing.T) string {
	t.Helper()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "checkout", "-b", "other")
	temp_repo.WriteFile(t, dir, "README.md", "from other\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: other side")
	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.WriteFile(t, dir, "README.md", "from main\n")
	temp_repo.RunGit(t, dir, "commit", "-am", "chore: main side")
	return dir
}
