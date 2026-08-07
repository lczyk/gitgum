package commands

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
)

func plainSummary(t *testing.T, files []git.DiffFile, width int) []string {
	t.Helper()
	return strings.Split(renderDiffSummary(files, width, false), "\n")
}

func TestRenderDiffSummary_Layout(t *testing.T) {
	t.Parallel()
	lines := plainSummary(t, []git.DiffFile{
		{Path: "added.txt", OldMode: git.ModeAbsent, NewMode: git.ModeFile, Status: 'A', Added: 1},
		{Path: "gone.txt", OldMode: git.ModeFile, NewMode: git.ModeAbsent, Status: 'D', Deleted: 1},
		{Path: "keep.txt", OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Added: 2, Deleted: 1},
	}, 80)

	assert.EqualArrays(t, lines, []string{
		" added.txt (new) | 1 +",
		" gone.txt (gone) | 1 -",
		" keep.txt        | 3 ++-",
		" 3 files changed, 3 insertions(+), 2 deletions(-)",
	})
}

// git drops a zero half unless both are zero, when saying nothing changed
// twice reads better than saying nothing at all.
func TestSummaryTotals(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		files, added, deleted int
		want                  string
	}{
		"one of each":     {1, 1, 1, "1 file changed, 1 insertion(+), 1 deletion(-)"},
		"insertions only": {1, 1, 0, "1 file changed, 1 insertion(+)"},
		"deletions only":  {1, 0, 1, "1 file changed, 1 deletion(-)"},
		"neither":         {1, 0, 0, "1 file changed, 0 insertions(+), 0 deletions(-)"},
		"plurals":         {3, 4, 2, "3 files changed, 4 insertions(+), 2 deletions(-)"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, summaryTotals(tt.files, tt.added, tt.deleted), tt.want)
		})
	}
}

func TestModeNote(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		old, new string
		want     string
	}{
		"new file":        {git.ModeAbsent, git.ModeFile, "(new)"},
		"new executable":  {git.ModeAbsent, git.ModeExec, "(new +x)"},
		"new symlink":     {git.ModeAbsent, git.ModeSymlink, "(new +l)"},
		"deleted":         {git.ModeFile, git.ModeAbsent, "(gone)"},
		"gained exec bit": {git.ModeFile, git.ModeExec, "(mode +x)"},
		"lost exec bit":   {git.ModeExec, git.ModeFile, "(mode -x)"},
		"became symlink":  {git.ModeFile, git.ModeSymlink, "(mode +l)"},
		"ordinary edit":   {git.ModeFile, git.ModeFile, ""},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, modeNote(git.DiffFile{OldMode: tt.old, NewMode: tt.new}), tt.want)
		})
	}
}

// The parts both paths share sit outside the braces, so what actually moved is
// what stands out.
func TestRenamePath(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ from, to, want string }{
		"across directories":  {"src/alpha/x.go", "src/beta/x.go", "src/{alpha => beta}/x.go"},
		"within a directory":  {"src/a/one.go", "src/a/two.go", "src/a/{one.go => two.go}"},
		"nothing in common":   {"one.txt", "other.txt", "one.txt => other.txt"},
		"prefix and suffix":   {"a/b/x/f.go", "a/c/x/f.go", "a/{b => c}/x/f.go"},
		"partial name shared": {"src/alpha/x.go", "src/alto/x.go", "src/{alpha => alto}/x.go"},
		// git leaves these two flat rather than bracing an empty side.
		"out of a directory": {"src/deep/f.go", "f.go", "src/deep/f.go => f.go"},
		"into a directory":   {"top.go", "src/deep/top.go", "top.go => src/deep/top.go"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, renamePath(tt.from, tt.to), tt.want)
		})
	}
}

// A rename is one row naming both ends, not two rows naming one end each.
func TestRenderDiffSummary_Rename(t *testing.T) {
	t.Parallel()
	lines := plainSummary(t, []git.DiffFile{{
		Path: "src/beta/x.go", RenamedFrom: "src/alpha/x.go",
		OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'R',
	}}, 80)
	assert.Equal(t, lines[0], " src/{alpha => beta}/x.go | 0")
}

// git counts no lines for a binary file, so there is nothing to draw a bar
// from and nothing to add to the totals.
func TestRenderDiffSummary_Binary(t *testing.T) {
	t.Parallel()
	lines := plainSummary(t, []git.DiffFile{
		{Path: "bin.dat", OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Binary: true},
		{Path: "keep.txt", OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Added: 1},
	}, 80)
	assert.Equal(t, lines[0], " bin.dat  | Bin")
	assert.Equal(t, lines[1], " keep.txt | 1 +")
	assert.Equal(t, lines[2], " 2 files changed, 1 insertion(+)")
}

// Bars are proportional to the largest change in the same diff, so the rows
// are comparable to each other. A file that changed at all keeps a cell.
func TestSummaryBar_Scales(t *testing.T) {
	t.Parallel()
	big := summaryRow{total: 400, added: 400}
	small := summaryRow{total: 1, added: 1}

	assert.Equal(t, len(summaryBar(big, 400, 40, false)), 40)
	assert.Equal(t, len(summaryBar(small, 400, 40, false)), 1)
	assert.Equal(t, summaryBar(summaryRow{}, 400, 40, false), "")

	// Under the available width the bar is one cell per line.
	assert.Equal(t, summaryBar(summaryRow{total: 3, added: 2, deleted: 1}, 3, 40, false), "++-")
}

// A side that changed anything gets at least one cell to say so with, even
// when its share rounds to nothing.
func TestSplit_KeepsBothSidesVisible(t *testing.T) {
	t.Parallel()
	plus, minus := split(999, 1, 10)
	assert.Equal(t, plus+minus, 10)
	assert.That(t, minus >= 1, "a deletion must show, got %d", minus)

	plus, minus = split(1, 999, 10)
	assert.Equal(t, plus+minus, 10)
	assert.That(t, plus >= 1, "an insertion must show, got %d", plus)

	plus, minus = split(5, 0, 4)
	assert.Equal(t, plus, 4)
	assert.Equal(t, minus, 0)
}

// A narrow terminal shortens the bars rather than wrapping the rows.
func TestRenderDiffSummary_NarrowWidth(t *testing.T) {
	t.Parallel()
	files := []git.DiffFile{{Path: "keep.txt", OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Added: 400}}
	wide := plainSummary(t, files, 100)[0]
	narrow := plainSummary(t, files, 30)[0]
	assert.That(t, len(narrow) < len(wide), "narrow %q should be shorter than wide %q", narrow, wide)
	assert.That(t, len(narrow) <= 30, "narrow row should fit the width, got %d", len(narrow))
}

// A deep path is shortened from the front: the leading directories are what
// the rows have in common, so they are what a reader least needs.
func TestElidePath(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		path  string
		width int
		want  string
	}{
		"fits":            {"src/x.go", 20, "src/x.go"},
		"exactly fits":    {"src/x.go", 8, "src/x.go"},
		"cuts on a slash": {"optfil/kernel-include/linux/xt.h", 28, ".../linux/xt.h"},
		"keeps the whole tail when it fits": {
			"optfil/kernel-include/linux/netfilter/xt_CONNMARK.h", 50,
			".../kernel-include/linux/netfilter/xt_CONNMARK.h",
		},
		"no slash to cut": {"averylongfilenamewithnodirs.txt", 12, "...odirs.txt"},
		"width too small": {"src/x.go", 3, "src/x.go"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, elidePath(tt.path, tt.width), tt.want)
		})
	}
}

// The path column is capped so a repo with deep paths still has room for the
// bars, which are the reason to draw a diffstat rather than list files.
func TestRenderDiffSummary_DeepPathsKeepTheirBars(t *testing.T) {
	t.Parallel()
	deep := "projects/linux-6.10.5/include/uapi/linux/netfilter/xt_CONNMARK.h"
	lines := plainSummary(t, []git.DiffFile{
		{Path: deep, OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Added: 40, Deleted: 5},
		{Path: "short.txt", OldMode: git.ModeFile, NewMode: git.ModeFile, Status: 'M', Added: 1},
	}, 80)

	assert.ContainsString(t, lines[0], "...")
	assert.ContainsString(t, lines[0], "+")
	for _, line := range lines {
		assert.That(t, len(line) <= 80, "row should fit the width, got %d: %q", len(line), line)
	}
}
