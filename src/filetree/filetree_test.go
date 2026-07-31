package filetree_test

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/filetree"
)

func render(items []filetree.Item, opt filetree.Opts) string {
	var b strings.Builder
	filetree.Tree(&b, items, opt)
	return b.String()
}

func paths(p ...string) []filetree.Item {
	out := make([]filetree.Item, len(p))
	for i, s := range p {
		out[i] = filetree.Item{Path: s}
	}
	return out
}

func TestTree(t *testing.T) {
	cases := map[string]struct {
		items []filetree.Item
		opt   filetree.Opts
		want  string
	}{
		"empty draws nothing": {
			items: nil,
			want:  "",
		},
		"blank paths are skipped": {
			items: paths("", "/", "go.mod"),
			want:  "go.mod\n",
		},
		"top level is flush and name-sorted": {
			items: paths("go.mod", "delete_me"),
			want: "delete_me\n" +
				"go.mod\n",
		},
		"children carry box drawing": {
			items: []filetree.Item{
				{Path: ".github/scripts", Prefix: "[??] ", IsDir: true},
				{Path: "spread.yaml", Prefix: "[ M] ", Suffix: " (+146,-109)"},
			},
			want: ".github/\n" +
				"└─ [??] scripts/\n" +
				"[ M] spread.yaml (+146,-109)\n",
		},
		"siblings branch, last one closes": {
			items: paths("a/1", "a/2", "a/3"),
			want: "a/\n" +
				"├─ 1\n" +
				"├─ 2\n" +
				"└─ 3\n",
		},
		"depth pads under the branch it hangs from": {
			items: paths("a/b/1", "a/c/1"),
			want: "a/\n" +
				"├─ b/\n" +
				"│  └─ 1\n" +
				"└─ c/\n" +
				"   └─ 1\n",
		},
		"unfolded chain gets a line per level": {
			items: paths(
				".github/scripts/install-slices/__pycache__/conftest.pyc",
				".github/scripts/install-slices/__pycache__/install_slices.pyc",
			),
			want: ".github/\n" +
				"└─ scripts/\n" +
				"   └─ install-slices/\n" +
				"      └─ __pycache__/\n" +
				"         ├─ conftest.pyc\n" +
				"         └─ install_slices.pyc\n",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualLineByLine(t, render(tt.items, tt.opt), tt.want)
		})
	}
}

func TestTree_FoldChains(t *testing.T) {
	cases := map[string]struct {
		items []filetree.Item
		want  string
	}{
		"chain collapses to one segment": {
			items: paths(
				".github/scripts/install-slices/__pycache__/conftest.pyc",
				".github/scripts/install-slices/__pycache__/install_slices.pyc",
			),
			want: ".github/scripts/install-slices/__pycache__/\n" +
				"├─ conftest.pyc\n" +
				"└─ install_slices.pyc\n",
		},
		"a lone file folds all the way to its path": {
			items: []filetree.Item{{Path: "a/b/c.txt", Prefix: "[ M] "}},
			want:  "[ M] a/b/c.txt\n",
		},
		"a directory that is itself an item stops the run": {
			items: []filetree.Item{
				{Path: "a", Prefix: "[??] ", IsDir: true},
				{Path: "a/b/c.txt"},
			},
			want: "[??] a/\n" +
				"└─ b/c.txt\n",
		},
		"a fork stops the run": {
			items: paths("a/b/1", "a/c/1"),
			want: "a/\n" +
				"├─ b/1\n" +
				"└─ c/1\n",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			got := render(tt.items, filetree.Opts{FoldChains: true})
			assert.EqualLineByLine(t, got, tt.want)
		})
	}
}

func TestTree_FoldOver(t *testing.T) {
	items := paths("node_modules/dep/1", "node_modules/dep/2", "node_modules/dep/3", "go.mod")

	got := render(items, filetree.Opts{FoldOver: 2})
	assert.EqualLineByLine(t, got, "go.mod\nnode_modules/ (3 files)\n")

	// Under the threshold nothing folds, and no count line appears.
	got = render(items, filetree.Opts{FoldOver: 10})
	assert.EqualLineByLine(t, got,
		"go.mod\n"+
			"node_modules/\n"+
			"└─ dep/\n"+
			"   ├─ 1\n"+
			"   ├─ 2\n"+
			"   └─ 3\n")
}

// A folded directory reports the leaves it stands for, so the trailing budget
// line must not count them again.
func TestTree_FoldOverIsNotDoubleCounted(t *testing.T) {
	items := paths("a/1", "a/2", "a/3", "b/1", "b/2")
	got := render(items, filetree.Opts{FoldOver: 1})
	assert.EqualLineByLine(t, got, "a/ (3 files)\nb/ (2 files)\n")
}

func TestTree_MaxChildren(t *testing.T) {
	items := paths("a/1", "a/2", "a/3", "a/4", "a/5")
	got := render(items, filetree.Opts{MaxChildren: 2})
	assert.EqualLineByLine(t, got,
		"a/\n"+
			"├─ 1\n"+
			"├─ 2\n"+
			"└─ ... and 3 more\n")
}

// At the top level there is no box drawing to hang the line off, so it sits
// flush -- the same shape the budget line uses, since neither has a parent.
func TestTree_MaxChildrenAtTopLevel(t *testing.T) {
	got := render(paths("1", "2", "3"), filetree.Opts{MaxChildren: 2})
	assert.EqualLineByLine(t, got, "1\n2\n... and 1 more\n")
}

func TestTree_MaxLines(t *testing.T) {
	cases := map[string]struct {
		items []filetree.Item
		opt   filetree.Opts
		want  string
	}{
		"flat list stops and says how many are left": {
			items: paths("1", "2", "3", "4", "5"),
			opt:   filetree.Opts{MaxLines: 3},
			want:  "1\n2\n3\n... and 2 more\n",
		},
		"a cut mid-subtree counts the siblings it never reached": {
			items: paths("a/1", "a/2", "a/3", "b/1"),
			opt:   filetree.Opts{MaxLines: 3},
			want: "a/\n" +
				"├─ 1\n" +
				"├─ 2\n" +
				"... and 2 more\n",
		},
		"directory lines spend budget without being leaves": {
			items: paths("a/1", "a/2"),
			opt:   filetree.Opts{MaxLines: 1},
			want:  "a/\n... and 2 more\n",
		},
		"an exact fit says nothing extra": {
			items: paths("1", "2", "3"),
			opt:   filetree.Opts{MaxLines: 3},
			want:  "1\n2\n3\n",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualLineByLine(t, render(tt.items, tt.opt), tt.want)
		})
	}
}

// The two caps report disjoint sets: whatever a per-directory line already
// accounted for is not counted again by the budget line.
func TestTree_BothCapsCountDisjointly(t *testing.T) {
	items := paths("a/1", "a/2", "a/3", "a/4", "a/5", "b/1")

	// Budget runs out before the per-directory line lands, so it reports nothing
	// and all four undrawn leaves fall to the trailing line.
	got := render(items, filetree.Opts{MaxChildren: 2, MaxLines: 3})
	assert.EqualLineByLine(t, got,
		"a/\n"+
			"├─ 1\n"+
			"├─ 2\n"+
			"... and 4 more\n")

	// One more line of budget lets it land, and the trailing count drops to the
	// single leaf under b/ that was never reached.
	got = render(items, filetree.Opts{MaxChildren: 2, MaxLines: 4})
	assert.EqualLineByLine(t, got,
		"a/\n"+
			"├─ 1\n"+
			"├─ 2\n"+
			"└─ ... and 3 more\n"+
			"... and 1 more\n")
}

func TestTree_Dim(t *testing.T) {
	got := render(paths("a/b"), filetree.Opts{Dim: func(s string) string { return "<" + s + ">" }})
	assert.EqualLineByLine(t, got, "a/\n<└─ >b\n")
}

func TestTree_QuotesAwkwardNames(t *testing.T) {
	got := render(paths("dir name/sp ace.txt"), filetree.Opts{})
	assert.EqualLineByLine(t, got, "\"dir name\"/\n└─ \"sp ace.txt\"\n")

	// Folding joins quoted segments, so the slashes stay structural.
	got = render(paths("dir name/sp ace.txt"), filetree.Opts{FoldChains: true})
	assert.EqualLineByLine(t, got, "\"dir name\"/\"sp ace.txt\"\n")
}

func TestFlat(t *testing.T) {
	items := []filetree.Item{
		{Path: "pkg/go.mod", Prefix: " M "},
		{Path: "sp ace.txt", Prefix: "?? "},
		{Path: "untracked_dir", Prefix: "?? ", IsDir: true},
	}
	var b strings.Builder
	filetree.Flat(&b, items, filetree.Opts{})
	assert.EqualLineByLine(t, b.String(),
		" M pkg/go.mod\n"+
			"?? \"sp ace.txt\"\n"+
			"?? untracked_dir/\n")
}

func TestFlat_MaxLines(t *testing.T) {
	var b strings.Builder
	filetree.Flat(&b, paths("1", "2", "3", "4"), filetree.Opts{MaxLines: 2})
	assert.EqualLineByLine(t, b.String(), "1\n2\n... and 2 more\n")
}

func TestQuoteName(t *testing.T) {
	cases := map[string]string{
		"plain.txt":     "plain.txt",
		"has space.txt": `"has space.txt"`,
		"has\ttab":      `"has\ttab"`,
		`has"quote`:     `"has\"quote"`,
		`has\backslash`: `"has\\backslash"`,
		"h\x01ctrl":     `"h\x01ctrl"`,
		"café.txt":      "café.txt",
		"日本.txt":        "日本.txt",
		"":              "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, filetree.QuoteName(in), want)
		})
	}
}
