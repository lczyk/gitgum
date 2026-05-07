package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// runBoth executes `gg diff` once via the passthrough backend and once via the
// native backend, returning both captured outputs. byte-identity between the
// two is the contract -- assert in caller.
func runBoth(t *testing.T, repo git.Repo) (passthrough, native string) {
	t.Helper()

	var buf bytes.Buffer
	cmd := &DiffCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}}

	t.Setenv("GG_DIFF_NATIVE", "0")
	assert.NoError(t, cmd.Execute(nil))
	passthrough = buf.String()

	buf.Reset()
	t.Setenv("GG_DIFF_NATIVE", "1")
	assert.NoError(t, cmd.Execute(nil))
	native = buf.String()
	return
}

// assertParity is the contract: native and passthrough produce byte-identical
// output (ANSI included).
func assertParity(t *testing.T, repo git.Repo) string {
	t.Helper()
	pt, nt := runBoth(t, repo)
	assert.Equal(t, pt, nt)
	return pt
}

func TestDiffCommand_Parity_EmptyDiff(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.Equal(t, out, "")
}

func TestDiffCommand_Parity_ModifiedFile(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "hello\n", "chore: add a")
	temp_repo.WriteFile(t, dir, "a.txt", "hello world\n")
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.ContainsString(t, out, "a.txt")
}

func TestDiffCommand_Parity_UntrackedFileShowsEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "new.txt", "fresh\n")
	repo := git.Repo{Dir: dir}

	// `git diff` (no --cached) ignores untracked files, so output is empty.
	// the point of this case is parity in the empty-but-not-trivially-empty path.
	out := assertParity(t, repo)
	assert.Equal(t, out, "")
}

func TestDiffCommand_Parity_DeletedFile(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "doomed.txt", "bye\n", "chore: add doomed")
	assert.NoError(t, os.Remove(filepath.Join(dir, "doomed.txt")))
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.ContainsString(t, out, "doomed.txt")
}

func TestDiffCommand_Parity_ModeChange(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "script.sh", "echo hi\n", "chore: add script")
	assert.NoError(t, os.Chmod(filepath.Join(dir, "script.sh"), 0o755))
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.ContainsString(t, out, "script.sh")
}

func TestDiffCommand_Parity_Rename(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "old.txt", "content\n", "chore: add old")
	temp_repo.RunGit(t, dir, "mv", "old.txt", "new.txt")
	repo := git.Repo{Dir: dir}

	// just exercise parity -- compact-summary may render this as add+del or
	// as a rename depending on git's similarity heuristic; either way both
	// backends must agree.
	_ = assertParity(t, repo)
}

func TestDiffCommand_Parity_BinaryModified(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	bin := []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0x7f, 0x00}
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "blob.bin"), bin, 0o644))
	temp_repo.RunGit(t, dir, "add", "blob.bin")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: add blob")
	// modify the binary in the working tree so it shows up in `git diff`.
	bin2 := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x11, 0x22, 0x33}
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "blob.bin"), bin2, 0o644))
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.ContainsString(t, out, "blob.bin")
}

func TestDiffCommand_Parity_MultiFileMix(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "keep.txt", "one\ntwo\nthree\n", "chore: add keep")
	temp_repo.CreateCommit(t, dir, "drop.txt", "gone\n", "chore: add drop")
	// modify keep.txt
	temp_repo.WriteFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\n")
	// delete drop.txt
	assert.NoError(t, os.Remove(filepath.Join(dir, "drop.txt")))
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	assert.ContainsString(t, out, "keep.txt")
	assert.ContainsString(t, out, "drop.txt")
}

func TestDiffCommand_Parity_StagedOnlyShowsEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "staged.txt", "stuff\n")
	temp_repo.RunGit(t, dir, "add", "staged.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: add staged")
	// now stage a change but the working tree matches the index
	temp_repo.WriteFile(t, dir, "staged.txt", "more stuff\n")
	temp_repo.RunGit(t, dir, "add", "staged.txt")
	repo := git.Repo{Dir: dir}

	// `gg diff` shows working-tree-vs-index, so a fully-staged change is empty.
	out := assertParity(t, repo)
	assert.Equal(t, out, "")
}

func TestDiffCommand_Parity_WithColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "hello\n", "chore: add a")
	temp_repo.WriteFile(t, dir, "a.txt", "hello world\n")
	repo := git.Repo{Dir: dir}

	out := assertParity(t, repo)
	// FORCE_COLOR=1 + non-empty diff should contain ANSI.
	assert.That(t, len(out) > 0, "expected non-empty output with FORCE_COLOR")
}

func TestDiffCommand_RejectsArgs(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	repo := git.Repo{Dir: dir}
	var buf bytes.Buffer
	cmd := &DiffCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}}
	err := cmd.Execute([]string{"unexpected"})
	assert.That(t, err != nil, "expected error for positional args")
}
