package commands

import (
	"regexp"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/src/filetree"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

func entry(x, y byte, path string) git.Entry { return git.Entry{X: x, Y: y, Path: path} }

// drawStatus is renderChanges' tree half, so the cases assert what reaches the
// terminal rather than the item structs on the way there.
func drawStatus(entries []git.Entry, stats map[string]numstat) string {
	var buf strings.Builder
	filetree.Tree(&buf, statusItems(entries, stats), filetree.Opts{Dim: dim})
	return stripAnsi(buf.String())
}

func TestStatusItems(t *testing.T) {
	cases := map[string]struct {
		entries []git.Entry
		stats   map[string]numstat
		want    string
	}{
		"name-sorted, code on the left": {
			entries: []git.Entry{entry(' ', 'D', "go.mod"), entry('?', '?', "delete_me")},
			want: "[??] delete_me\n" +
				"[ D] go.mod\n",
		},
		"a diffstat lands on the right, and only where there is one": {
			entries: []git.Entry{entry(' ', 'M', "go.mod"), entry('?', '?', "untracked.txt")},
			stats:   map[string]numstat{"go.mod": {added: 4, deleted: 1}},
			want: "[ M] go.mod (+4,-1)\n" +
				"[??] untracked.txt\n",
		},
		"nested paths become a tree": {
			entries: []git.Entry{
				entry(' ', 'M', "internal/git/git.go"),
				entry('?', '?', "src/commands/foo.go"),
				entry(' ', 'D', "go.mod"),
			},
			want: "[ D] go.mod\n" +
				"internal/\n" +
				"└─ git/\n" +
				"   └─ [ M] git.go\n" +
				"src/\n" +
				"└─ commands/\n" +
				"   └─ [??] foo.go\n",
		},
		"a rename draws both halves": {
			entries: []git.Entry{{X: 'R', Y: ' ', Path: "b/new.go", RenamedFrom: "a/old.go"}},
			want: "a/\n" +
				"└─ [R<] old.go\n" +
				"b/\n" +
				"└─ [R>] new.go\n",
		},
		// git reports an entirely-untracked directory as one entry, and the
		// slash is the only thing saying it stands for a subtree.
		"an untracked directory keeps its slash": {
			entries: []git.Entry{entry('?', '?', ".github/scripts/")},
			want: ".github/\n" +
				"└─ [??] scripts/\n",
		},
		"an awkward name is quoted": {
			entries: []git.Entry{entry('?', '?', "sp ace.txt")},
			want:    "[??] \"sp ace.txt\"\n",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualLineByLine(t, drawStatus(tt.entries, tt.stats), tt.want)
		})
	}
}

// --flat offers git's own shape, so a rename stays on one line and nothing is
// coloured -- but the quoting is gg's, since git's C-quoting is what -z avoids.
func TestFlatItems(t *testing.T) {
	entries := []git.Entry{
		entry(' ', 'M', "go.mod"),
		{X: 'R', Y: ' ', Path: "b/new.go", RenamedFrom: "a/old.go"},
		entry('?', '?', "sp ace.txt"),
		entry('?', '?', "untracked_dir/"),
	}
	var buf strings.Builder
	filetree.Flat(&buf, flatItems(entries), filetree.Opts{})
	assert.EqualLineByLine(t, buf.String(),
		" M go.mod\n"+
			"R  a/old.go -> b/new.go\n"+
			"?? \"sp ace.txt\"\n"+
			"?? untracked_dir/\n")
}

func TestParseNumstat(t *testing.T) {
	in := "12\t3\tfoo.go\n0\t5\tbar.go\n-\t-\timg.png\n7\t1\tnested/baz.go\n"
	got := parseNumstat(in)
	assert.Equal(t, len(got), 3) // binary line skipped
	assert.Equal(t, got["foo.go"].added, 12)
	assert.Equal(t, got["foo.go"].deleted, 3)
	assert.Equal(t, got["bar.go"].added, 0)
	assert.Equal(t, got["bar.go"].deleted, 5)
	assert.Equal(t, got["nested/baz.go"].added, 7)
}

func TestFormatNumstat(t *testing.T) {
	got := stripAnsi(formatNumstat(numstat{added: 5, deleted: 2}))
	assert.Equal(t, got, "(+5,-2)")
}
