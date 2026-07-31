package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/dirty"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
)

func newDirtyTestIO(stub *stubSelector, dir string) *cmdIO {
	return &cmdIO{Out: &strings.Builder{}, Err: &strings.Builder{}, UI: stub, Repo: git.Repo{Dir: dir}}
}

func TestHandleDirtyTree_Clean(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	stub := &stubSelector{}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	require.NoError(t, err)
	assert.That(t, cleanup != nil, "cleanup must always be non-nil")
	cleanup()
	assert.Equal(t, len(stub.selectCalls), 0)
}

func TestHandleDirtyTree_UntrackedOnly(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	require.NoError(t, err)

	stub := &stubSelector{}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	require.NoError(t, err)
	cleanup()
	assert.Equal(t, len(stub.selectCalls), 0)

	// No stash should have been created.
	stashList := temp_repo.RunGit(t, dir, "stash", "list")
	assert.Equal(t, strings.TrimSpace(stashList), "")
}

func TestHandleDirtyTree_DirtyConfirmYes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	readme := filepath.Join(dir, "README.md")
	err := os.WriteFile(readme, []byte("modified\n"), 0o644)
	require.NoError(t, err)

	stub := &stubSelector{selectAnswers: []string{dirtyStashOption("release")}}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "release")
	require.NoError(t, err)
	assert.Equal(t, len(stub.selectCalls), 1)
	assert.ContainsString(t, stub.selectCalls[0].Options[1], "release")

	// Tree should now be clean (changes stashed).
	out := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.Equal(t, strings.TrimSpace(out), "")

	// Stash list entry uses the label.
	stashList := temp_repo.RunGit(t, dir, "stash", "list")
	assert.ContainsString(t, stashList, "gitgum release auto-stash")

	cleanup()

	// After cleanup, tree should be dirty again and stash empty.
	post := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, post, "README.md")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}

// Regression: untracked files coexisting with tracked changes must still
// trigger the prompt. Earlier porcelain parsing risked confusing the leading
// space of " M" status codes with the "?? " untracked prefix.
func TestHandleDirtyTree_TrackedAndUntrackedMixed(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)
	require.NoError(t, err)

	stub := &stubSelector{selectAnswers: []string{dirtyStashOption("test")}}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, len(stub.selectCalls), 1)

	// Untracked file must remain in working tree.
	_, statErr := os.Stat(filepath.Join(dir, "untracked.txt"))
	require.NoError(t, statErr, "untracked file should not be stashed away")
}

func TestHandleDirtyTree_RestoresIndexAfterPop(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	stagedPath := filepath.Join(dir, "staged.txt")
	err := os.WriteFile(stagedPath, []byte("staged\n"), 0o644)
	require.NoError(t, err)
	temp_repo.RunGit(t, dir, "add", "staged.txt")

	unstagedPath := filepath.Join(dir, "README.md")
	err = os.WriteFile(unstagedPath, []byte("changed\n"), 0o644)
	require.NoError(t, err)

	stub := &stubSelector{selectAnswers: []string{dirtyStashOption("test")}}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	require.NoError(t, err)
	cleanup()

	out := temp_repo.RunGit(t, dir, "status", "--porcelain")
	gotStaged := map[string]string{}
	for l := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if len(l) < 4 {
			continue
		}
		gotStaged[l[3:]] = l[:2]
	}
	assert.Equal(t, gotStaged["staged.txt"], "A ")
	assert.Equal(t, gotStaged["README.md"], " M")

	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
}

// Partial-hunk staging: a file has both staged and unstaged changes (XY=MM
// in porcelain). After stash + pop --index, the same byte-for-byte split
// between index and worktree must hold.
func TestHandleDirtyTree_PreservesPartialHunkStaging(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	path := filepath.Join(dir, "file.txt")
	err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644)
	require.NoError(t, err)
	temp_repo.RunGit(t, dir, "add", "file.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: add file")

	err = os.WriteFile(path, []byte("A\nb\nc\nd\nE\n"), 0o644)
	require.NoError(t, err)
	stagePatch := "" +
		"diff --git a/file.txt b/file.txt\n" +
		"--- a/file.txt\n" +
		"+++ b/file.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		"-a\n" +
		"+A\n" +
		" b\n" +
		" c\n"
	patchPath := filepath.Join(dir, "stage.patch")
	err = os.WriteFile(patchPath, []byte(stagePatch), 0o644)
	require.NoError(t, err)
	temp_repo.RunGit(t, dir, "apply", "--cached", patchPath)
	os.Remove(patchPath)

	pre := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, pre, "MM file.txt")

	indexBefore := temp_repo.RunGit(t, dir, "show", ":file.txt")
	worktreeBefore, err := os.ReadFile(path)
	require.NoError(t, err)

	stub := &stubSelector{selectAnswers: []string{dirtyStashOption("test")}}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	require.NoError(t, err)
	cleanup()

	indexAfter := temp_repo.RunGit(t, dir, "show", ":file.txt")
	worktreeAfter, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, indexAfter, indexBefore)
	assert.Equal(t, string(worktreeAfter), string(worktreeBefore))

	post := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, post, "MM file.txt")
}

func TestHandleDirtyTree_DirtyConfirmNo(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	readme := filepath.Join(dir, "README.md")
	err := os.WriteFile(readme, []byte("modified\n"), 0o644)
	require.NoError(t, err)

	stub := &stubSelector{selectAnswers: []string{dirtyAbort}}
	c := newDirtyTestIO(stub, dir)

	cleanup, err := handleDirtyTree(c, "test")
	assert.Error(t, err, assert.AnyError, "should error when user declines")
	assert.That(t, errors.Is(err, errDirtyTreeAborted), "abort should match errDirtyTreeAborted")
	assert.That(t, cleanup != nil, "cleanup must always be non-nil")
	cleanup()

	out := temp_repo.RunGit(t, dir, "status", "--porcelain")
	assert.ContainsString(t, out, "README.md")
}

// The row this prompt gained: discarding clears both the tracked changes the
// listing showed and the untracked files it did not.
func TestHandleDirtyTree_DiscardRemovesTrackedAndUntracked(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "edited\n")
	temp_repo.WriteFile(t, dir, "loose.txt", "new\n")

	var out strings.Builder
	stub := &stubSelector{}
	c := newDirtyTestIO(stub, dir)
	c.Out = &out
	// the discard row is last, and states its own counts
	stub.selectAnswers = []string{discardLabel(
		dirty.Plan{Tracked: []string{"README.md"}, Untracked: []string{"loose.txt"}},
		dirty.Options{Tracked: true, Untracked: true})}

	cleanup, err := handleDirtyTree(c, "switch")
	require.NoError(t, err, "discard should not error")
	defer cleanup()

	require.Equal(t, len(stub.selectCalls), 1)
	assert.Equal(t, len(stub.selectCalls[0].Options), 3)
	assert.Equal(t, stub.selectCalls[0].Options[0], dirtyAbort)
	assert.ContainsString(t, stub.selectCalls[0].Options[2], "1 change(s) and 1 untracked file(s)")
	assert.ContainsString(t, stub.selectCalls[0].Options[2], "cannot be undone")

	// both are gone, and nothing was stashed
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "status", "--porcelain")), "")
	assert.Equal(t, strings.TrimSpace(temp_repo.RunGit(t, dir, "stash", "list")), "")
	assert.ContainsString(t, out.String(), "Discarded 2 file(s)")
}

// Cancelling the picker is the same outcome as choosing the abort row.
func TestHandleDirtyTree_CancelAborts(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "README.md", "edited\n")

	stub := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	_, err := handleDirtyTree(newDirtyTestIO(stub, dir), "switch")
	assert.ErrorIs(t, err, errDirtyTreeAborted, "cancelling should abort cleanly")
}
