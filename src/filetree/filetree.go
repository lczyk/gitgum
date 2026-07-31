// Package filetree draws a list of slash-separated paths as an ascii tree.
//
// It knows nothing about git, and nothing about the filesystem: it renders the
// paths it is handed and no others, and every count it prints counts those.
// A folded directory says how many of the given paths it stands for, never how
// many files exist on disk -- a caller that lists a subset must not present the
// result as a complete one.
//
// Names are the package's to draw. Callers decorate them with Prefix and
// Suffix rather than supplying whole lines, because folding rewrites the name
// (a chain of single-child directories becomes one segment) and quoting may
// change it further, neither of which a precomposed line could survive.
//
// The two truncation lines are deliberately distinguishable. An indented
// "└─ ... and N more" means that directory has more children than MaxChildren
// allows. A flush "... and N more" at the end means the MaxLines budget ran
// out. Both count leaves, and neither counts a leaf the other reported.
package filetree

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Item is one path to draw. Prefix and Suffix bracket the name the renderer
// produces, so a status code lands left of it and a diffstat right of it. Both
// may carry ansi escapes; the renderer never inspects them.
type Item struct {
	Path   string
	Prefix string
	Suffix string
	IsDir  bool // draw a trailing slash: the path stands for a directory
}

// Opts configures one render. The zero value draws an unbounded, unfolded tree
// with no colour.
type Opts struct {
	// Dim paints the box-drawing characters and the parenthetical on a folded
	// directory. Nil leaves them bare.
	Dim func(string) string
	// FoldChains draws a run of single-child directories as one segment:
	// "a/b/c/" rather than three nested lines.
	FoldChains bool
	// FoldOver replaces a directory holding more than this many leaves with
	// "dir/ (N files)". Zero never folds.
	FoldOver int
	// MaxLines bounds the whole render. Zero is unlimited.
	MaxLines int
	// MaxChildren bounds each directory's drawn children. Zero is unlimited.
	MaxChildren int
}

// Tree draws items as a tree. Top-level entries sit flush against the left
// margin; everything below them carries box-drawing.
func Tree(w io.Writer, items []Item, opt Opts) {
	root := build(items)
	if opt.FoldChains {
		root = fold(root)
	}
	r := newRenderer(w, opt, root.leaves)
	r.children(root, "", true)
	r.finish()
}

// Flat draws one line per item, full path each time. It honours MaxLines and
// quotes names the same way Tree does; the fold and MaxChildren options have
// nothing to act on and are ignored.
func Flat(w io.Writer, items []Item, opt Opts) {
	r := newRenderer(w, opt, len(items))
	for i := range items {
		name := renderName(strings.Trim(items[i].Path, "/"), items[i].IsDir)
		if !r.emit(items[i].Prefix + name + items[i].Suffix) {
			break
		}
		r.drawn++
	}
	r.finish()
}

type node struct {
	name     string
	children map[string]*node
	item     *Item
	leaves   int
}

// build turns paths into a tree. A path lands on the node its last segment
// names, so an entry for a directory and entries for files inside it can
// coexist -- the directory node then carries an item and children both.
func build(items []Item) *node {
	root := &node{children: map[string]*node{}}
	for i := range items {
		path := strings.Trim(items[i].Path, "/")
		if path == "" {
			continue
		}
		cur := root
		parts := strings.Split(path, "/")
		for j, seg := range parts {
			child, ok := cur.children[seg]
			if !ok {
				child = &node{name: seg, children: map[string]*node{}}
				cur.children[seg] = child
			}
			if j == len(parts)-1 {
				child.item = &items[i]
			}
			cur = child
		}
	}
	countLeaves(root)
	return root
}

// countLeaves fills in how many items each subtree holds. A directory that is
// itself an item counts as one, on top of whatever is under it.
func countLeaves(n *node) int {
	total := 0
	if n.item != nil {
		total++
	}
	for _, c := range n.children {
		total += countLeaves(c)
	}
	n.leaves = total
	return total
}

// fold collapses runs of single-child directories, returning the node that
// takes the folded run's place. A node carrying an item stops the run: its
// decoration belongs to that name, and absorbing a child past it would draw
// the decoration against the wrong one.
func fold(n *node) *node {
	if len(n.children) > 0 {
		folded := make(map[string]*node, len(n.children))
		for _, c := range n.children {
			f := fold(c)
			folded[f.name] = f
		}
		n.children = folded
	}
	for n.name != "" && n.item == nil && len(n.children) == 1 {
		for _, only := range n.children {
			only.name = n.name + "/" + only.name
			n = only
		}
	}
	return n
}

type renderer struct {
	w   io.Writer
	opt Opts
	dim func(string) string

	lines    int // drawn so far, against MaxLines
	drawn    int // leaves given their own line
	reported int // leaves accounted for by a fold or a per-directory more line
	total    int
	stopped  bool
}

func newRenderer(w io.Writer, opt Opts, total int) *renderer {
	dim := opt.Dim
	if dim == nil {
		dim = func(s string) string { return s }
	}
	return &renderer{w: w, opt: opt, dim: dim, total: total}
}

// emit writes one line, or reports that the budget is spent. Once it is, the
// walk unwinds without drawing anything further.
func (r *renderer) emit(s string) bool {
	if r.stopped {
		return false
	}
	if r.opt.MaxLines > 0 && r.lines >= r.opt.MaxLines {
		r.stopped = true
		return false
	}
	fmt.Fprintln(r.w, s)
	r.lines++
	return true
}

// children draws n's children in name order. top suppresses box-drawing, which
// is what keeps the outermost entries flush against the margin.
func (r *renderer) children(n *node, prefix string, top bool) {
	keys := sortedNames(n)
	for i, k := range keys {
		if r.stopped {
			return
		}
		if r.opt.MaxChildren > 0 && i == r.opt.MaxChildren {
			r.more(n, keys[i:], prefix, top)
			return
		}
		r.node(n.children[k], prefix, i == len(keys)-1, top)
	}
}

func (r *renderer) node(n *node, prefix string, last, top bool) {
	head, pad := prefix, prefix
	if !top {
		branch, nextPad := "├─ ", "│  "
		if last {
			branch, nextPad = "└─ ", "   "
		}
		head += r.dim(branch)
		pad += r.dim(nextPad)
	}

	if r.opt.FoldOver > 0 && n.item == nil && len(n.children) > 0 && n.leaves > r.opt.FoldOver {
		if r.emit(head + renderName(n.name, true) + " " + r.dim(fmt.Sprintf("(%d files)", n.leaves))) {
			r.reported += n.leaves
		}
		return
	}

	name := renderName(n.name, n.item == nil || n.item.IsDir)
	if n.item != nil {
		name = n.item.Prefix + name + n.item.Suffix
	}
	if !r.emit(head + name) {
		return
	}
	if n.item != nil {
		r.drawn++
	}
	r.children(n, pad, false)
}

// more closes a directory whose children ran past MaxChildren.
func (r *renderer) more(n *node, rest []string, prefix string, top bool) {
	m := 0
	for _, k := range rest {
		m += n.children[k].leaves
	}
	line := prefix
	if !top {
		line += r.dim("└─ ")
	}
	if r.emit(line + fmt.Sprintf("... and %d more", m)) {
		r.reported += m
	}
}

// finish states what the budget cut off. It is exempt from the budget itself --
// a truncated list that does not say so is the one output worth never emitting.
func (r *renderer) finish() {
	if m := r.total - r.drawn - r.reported; m > 0 {
		fmt.Fprintf(r.w, "... and %d more\n", m)
	}
}

// renderName quotes each segment of name and marks directories with a trailing
// slash. Segments are quoted individually so a fold's slashes stay structural.
func renderName(name string, isDir bool) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = QuoteName(p)
	}
	out := strings.Join(parts, "/")
	if isDir && out != "" {
		out += "/"
	}
	return out
}

func sortedNames(n *node) []string {
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
