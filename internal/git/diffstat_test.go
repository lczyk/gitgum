package git_test

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

// The raw records come first and the numstat records after, both NUL-separated,
// with a rename spending two fields on its paths in each half.
const rawAndNumstat = ":000000 100644 0000000 fa49b07 A\x00added.txt\x00" +
	":100644 100644 742c16a ee72c5d M\x00bin.dat\x00" +
	":100644 000000 286c5f5 0000000 D\x00gone.txt\x00" +
	":100644 100755 587be6b 587be6b M\x00mode.txt\x00" +
	":000000 120000 0000000 1764325 A\x00link.txt\x00" +
	":100644 100644 3367afd 3367afd R100\x00from.txt\x00to.txt\x00" +
	"1\t0\tadded.txt\x00" +
	"-\t-\tbin.dat\x00" +
	"0\t1\tgone.txt\x00" +
	"0\t0\tmode.txt\x00" +
	"1\t0\tlink.txt\x00" +
	"0\t0\t\x00from.txt\x00to.txt\x00"

func byPath(files []git.DiffFile) map[string]git.DiffFile {
	out := make(map[string]git.DiffFile, len(files))
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

func TestParseDiffFiles(t *testing.T) {
	t.Parallel()
	files, err := git.ParseDiffFiles(rawAndNumstat)
	require.NoError(t, err, "parsing raw+numstat")
	require.Equal(t, len(files), 6, "one row per file")
	got := byPath(files)

	added := got["added.txt"]
	assert.Equal(t, added.Status, byte('A'))
	assert.Equal(t, added.OldMode, git.ModeAbsent)
	assert.Equal(t, added.Added, 1)

	bin := got["bin.dat"]
	assert.Equal(t, bin.Binary, true)
	assert.Equal(t, bin.Added, 0)
	assert.Equal(t, bin.Deleted, 0)

	gone := got["gone.txt"]
	assert.Equal(t, gone.NewMode, git.ModeAbsent)
	assert.Equal(t, gone.Deleted, 1)

	assert.Equal(t, got["mode.txt"].NewMode, git.ModeExec)
	assert.Equal(t, got["link.txt"].NewMode, git.ModeSymlink)

	renamed := got["to.txt"]
	assert.Equal(t, renamed.Renamed(), true)
	assert.Equal(t, renamed.RenamedFrom, "from.txt")
	assert.Equal(t, renamed.Status, byte('R'))
}

// Order is git's, so the rows land in the order the caller will print them.
func TestParseDiffFiles_KeepsOrder(t *testing.T) {
	t.Parallel()
	files, err := git.ParseDiffFiles(rawAndNumstat)
	require.NoError(t, err, "parsing raw+numstat")
	var names []string
	for _, f := range files {
		names = append(names, f.Path)
	}
	assert.EqualArrays(t, names, []string{"added.txt", "bin.dat", "gone.txt", "mode.txt", "link.txt", "to.txt"})
}

func TestParseDiffFiles_Empty(t *testing.T) {
	t.Parallel()
	files, err := git.ParseDiffFiles("")
	require.NoError(t, err, "an empty diff is not an error")
	assert.Equal(t, len(files), 0)
}

func TestParseDiffFiles_Unreadable(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"raw record with no path":      ":100644 100644 aaa bbb M\x00",
		"rename with one path":         ":100644 100644 aaa bbb R100\x00from.txt\x00",
		"raw header missing fields":    ":100644 100644 M\x00path\x00",
		"numstat record missing a tab": ":100644 100644 aaa bbb M\x00p\x001 0 p\x00",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := git.ParseDiffFiles(raw)
			assert.Error(t, err, assert.AnyError, "should not parse")
		})
	}
}

// The parse has to survive what git actually emits, not only what the canned
// string says it emits.
func TestDiffFiles_AgainstGit(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "keep.txt", "a\nb\n", "chore: base")
	temp_repo.CreateCommit(t, dir, "moved.txt", "old\n", "chore: to move")

	temp_repo.WriteFile(t, dir, "keep.txt", "a\nb\nc\n")
	temp_repo.WriteFile(t, dir, "new.txt", "fresh\n")
	temp_repo.RunGit(t, dir, "mv", "moved.txt", "elsewhere.txt")
	temp_repo.RunGit(t, dir, "add", "-A")

	files, err := git.Repo{Dir: dir}.DiffFiles("--cached")
	require.NoError(t, err, "reading the staged diff")
	got := byPath(files)

	assert.Equal(t, got["keep.txt"].Added, 1)
	assert.Equal(t, got["new.txt"].OldMode, git.ModeAbsent)
	assert.Equal(t, got["elsewhere.txt"].RenamedFrom, "moved.txt")
}

func TestDiffFiles_CleanTree(t *testing.T) {
	t.Parallel()
	files, err := git.Repo{Dir: temp_repo.NewRepo(t)}.DiffFiles()
	require.NoError(t, err, "a clean tree is not an error")
	assert.Equal(t, len(files), 0)
}

// A path with a space would be C-quoted in line-based output; -z leaves it
// alone, which is the reason the reader asks for -z.
func TestDiffFiles_PathWithSpace(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "a file.txt", "x\n")
	temp_repo.RunGit(t, dir, "add", "-A")

	files, err := git.Repo{Dir: dir}.DiffFiles("--cached")
	require.NoError(t, err, "reading the staged diff")
	require.Equal(t, len(files), 1, "one file")
	assert.Equal(t, files[0].Path, "a file.txt")
	assert.That(t, !strings.Contains(files[0].Path, `"`), "path should not be quoted")
}
