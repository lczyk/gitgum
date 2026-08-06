package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestFormatDetachedRows_NoRefs(t *testing.T) {
	t.Parallel()
	rows := formatDetachedRows("cf10b5f", nil, false)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, rows[0], "* HEAD cf10b5f (no branch)")
}

func TestFormatDetachedRows_OneRef(t *testing.T) {
	t.Parallel()
	refs := []headRef{{name: "feat/x", remote: "lczyk", suffix: "~1"}}
	rows := formatDetachedRows("cf10b5f", refs, false)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, rows[0], "* HEAD cf10b5f (lczyk/)feat/x~1")
}

// A branch tip is just the ~0 case, not a special one.
func TestFormatDetachedRows_AtBranchTip(t *testing.T) {
	t.Parallel()
	rows := formatDetachedRows("cf10b5f", []headRef{{name: "main", suffix: "~0"}}, false)
	assert.Equal(t, rows[0], "* HEAD cf10b5f main~0")
}

func TestFormatDetachedRows_ManyRefs(t *testing.T) {
	t.Parallel()
	refs := []headRef{
		{name: "feat/x", remote: "lczyk", suffix: "~1"},
		{name: "feat/y", suffix: "~3"},
		{name: "feat/z", remote: "origin", suffix: "~5", kind: kindRemote},
		{name: "v1.2.0", suffix: "~4", kind: kindTag},
	}
	rows := formatDetachedRows("cf10b5f", refs, false)
	assert.Equal(t, len(rows), 5)
	assert.Equal(t, rows[0], "* HEAD cf10b5f")
	assert.Equal(t, rows[1], "    (lczyk/)feat/x~1")
	assert.Equal(t, rows[2], "    feat/y~3")
	assert.Equal(t, rows[3], "    (origin/)feat/z~5")
	assert.Equal(t, rows[4], "    v1.2.0~4")
}

// A merge can put HEAD off the ref's first-parent chain; then there is no
// honest "~N" to print and the bare name is all we say.
func TestFormatDetachedRows_NoSuffix(t *testing.T) {
	t.Parallel()
	rows := formatDetachedRows("cf10b5f", []headRef{{name: "main"}}, false)
	assert.Equal(t, rows[0], "* HEAD cf10b5f main")
}

func TestFormatDetachedRows_Color(t *testing.T) {
	t.Parallel()
	refs := []headRef{{name: "feat/x", remote: "lczyk", suffix: "~1"}}
	rows := formatDetachedRows("cf10b5f", refs, true)
	assert.ContainsString(t, rows[0], ansiBoldCyan+"HEAD"+ansiReset)
	assert.ContainsString(t, rows[0], ansiYellow+"cf10b5f"+ansiReset)
	assert.ContainsString(t, rows[0], ansiDim+"~1"+ansiReset)
	assert.Equal(t, stripAnsi(rows[0]), "* HEAD cf10b5f (lczyk/)feat/x~1")
}

func TestFormatDetachedRows_ColorNoRefsAndTag(t *testing.T) {
	t.Parallel()
	rows := formatDetachedRows("cf10b5f", nil, true)
	assert.ContainsString(t, rows[0], ansiBoldRed+"(no branch)"+ansiReset)

	rows = formatDetachedRows("cf10b5f", []headRef{{name: "v1", kind: kindTag, suffix: "~2"}}, true)
	assert.ContainsString(t, rows[0], ansiBoldMagenta+"v1"+ansiReset)
}

func TestFormatDetachedRows_ColorRemoteOnly(t *testing.T) {
	t.Parallel()
	refs := []headRef{{name: "feat/z", remote: "origin", suffix: "~2", kind: kindRemote}}
	rows := formatDetachedRows("cf10b5f", refs, true)
	assert.ContainsString(t, rows[0], ansiBoldRed+"origin/"+ansiReset)
	assert.ContainsString(t, rows[0], ansiBoldGreen+"feat/z"+ansiReset)
	assert.Equal(t, stripAnsi(rows[0]), "* HEAD cf10b5f (origin/)feat/z~2")
}

// A remote branch with no local counterpart still tells you where you are; a
// remote branch a local one already tracks is a duplicate and stays hidden.
func TestDetachedHeadRows_RemoteBranches(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "a.txt", "a\n")
	temp_repo.RunGit(t, dir, "add", "a.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "feat: a")
	temp_repo.RunGit(t, dir, "update-ref", "refs/remotes/origin/feat/z", "main")
	temp_repo.RunGit(t, dir, "update-ref", "refs/remotes/origin/main", "main")
	temp_repo.RunGit(t, dir, "config", "remote.origin.url", dir)
	temp_repo.RunGit(t, dir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	temp_repo.RunGit(t, dir, "config", "branch.main.remote", "origin")
	temp_repo.RunGit(t, dir, "config", "branch.main.merge", "refs/heads/main")
	temp_repo.RunGit(t, dir, "checkout", "HEAD~1")

	s := &StatusCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	rows, ok := s.detachedHeadRows(localBranchesIn(t, dir))
	assert.Equal(t, ok, true)
	assert.Equal(t, len(rows), 3)
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[1])), "(origin/)main~1")
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[2])), "(origin/)feat/z~1")
}

func TestDetachedHeadRows_Integration(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, "a.txt", "a\n")
	temp_repo.RunGit(t, dir, "add", "a.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "feat: a")
	temp_repo.RunGit(t, dir, "tag", "v1")
	temp_repo.RunGit(t, dir, "checkout", "HEAD~1")

	s := &StatusCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	rows, ok := s.detachedHeadRows(localBranchesIn(t, dir))
	assert.Equal(t, ok, true)
	assert.Equal(t, len(rows), 3)
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[1])), "main~1")
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[2])), "v1~1")

	// detached at the branch tip: same shape, ~0
	temp_repo.RunGit(t, dir, "checkout", "--detach", "main")
	rows, ok = s.detachedHeadRows(localBranchesIn(t, dir))
	assert.Equal(t, ok, true)
	assert.Equal(t, len(rows), 3)
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[1])), "main~0")
	assert.Equal(t, strings.TrimSpace(stripAnsi(rows[2])), "v1~0")

	// a commit no ref can reach
	temp_repo.WriteFile(t, dir, "b.txt", "b\n")
	temp_repo.RunGit(t, dir, "add", "b.txt")
	temp_repo.RunGit(t, dir, "commit", "-m", "feat: orphan")
	rows, ok = s.detachedHeadRows(localBranchesIn(t, dir))
	assert.Equal(t, ok, true)
	assert.Equal(t, len(rows), 1)
	assert.Equal(t, strings.HasSuffix(stripAnsi(rows[0]), " (no branch)"), true)
}

// localBranchesIn is the listing detachedHeadRows takes from the HEAD read in
// production; the tests take it themselves.
func localBranchesIn(t *testing.T, dir string) []git.LocalBranch {
	t.Helper()
	branches, err := git.Repo{Dir: dir}.LocalBranches()
	require.NoError(t, err, "listing local branches")
	return branches
}
