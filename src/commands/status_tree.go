package commands

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

type changeEntry struct {
	code    string // 2-char porcelain XY, or "R<"/"R>" for rename source/dest
	path    string
	numstat *numstat // nil if unavailable (untracked, binary, or no HEAD)
}

type numstat struct {
	added, deleted int
}

// annotateNumstats fills the numstat field on each entry whose path has a
// matching `git diff --numstat HEAD --no-renames` line. Untracked files,
// binary diffs, rename markers, and empty repos (no HEAD) get nil -- the
// renderer simply omits the count for those.
func annotateNumstats(repo git.Repo, entries []changeEntry) {
	out, _, err := repo.Run("diff", "--numstat", "--no-renames", "HEAD")
	if err != nil {
		return
	}
	stats := parseNumstat(out)
	for i := range entries {
		if ns, ok := stats[entries[i].path]; ok {
			entries[i].numstat = &ns
		}
	}
}

// parseNumstat parses `git diff --numstat` output into a path-keyed map.
// Binary diffs (added/deleted == "-") are skipped.
func parseNumstat(out string) map[string]numstat {
	m := map[string]numstat{}
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "-" || fields[1] == "-" {
			continue
		}
		a, errA := strconv.Atoi(fields[0])
		d, errD := strconv.Atoi(fields[1])
		if errA != nil || errD != nil {
			continue
		}
		m[fields[2]] = numstat{added: a, deleted: d}
	}
	return m
}

type treeNode struct {
	name     string
	children map[string]*treeNode
	entry    *changeEntry
}

func parseChangeLines(lines []string) []changeEntry {
	var out []changeEntry
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		rest := line[3:]
		if (code[0] == 'R' || code[1] == 'R') && strings.Contains(rest, " -> ") {
			parts := strings.SplitN(rest, " -> ", 2)
			out = append(out, changeEntry{code: "R<", path: parts[0]})
			out = append(out, changeEntry{code: "R>", path: parts[1]})
			continue
		}
		out = append(out, changeEntry{code: code, path: rest})
	}
	return out
}

func buildTree(entries []changeEntry) *treeNode {
	root := &treeNode{name: ".", children: map[string]*treeNode{}}
	for i := range entries {
		path := strings.TrimRight(entries[i].path, "/")
		if path == "" {
			continue
		}
		parts := strings.Split(path, "/")
		cur := root
		for j, p := range parts {
			child, ok := cur.children[p]
			if !ok {
				child = &treeNode{name: p, children: map[string]*treeNode{}}
				cur.children[p] = child
			}
			if j == len(parts)-1 {
				child.entry = &entries[i]
			}
			cur = child
		}
	}
	return root
}

const (
	ansiReset   = "\033[0m"
	ansiDim     = "\033[2m"
	ansiItalic  = "\033[3m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiOrange  = "\033[38;5;208m"
)

func dim(s string) string { return paint(ansiDim, s) }

// colorCodeChar colors one porcelain status char. side==0 is X (staged),
// side==1 is Y (worktree). Rules mirror git's own status colors.
func colorCodeChar(c byte, side int) string {
	s := string(c)
	switch c {
	case ' ':
		return dim(s)
	case '?', '!':
		return paint(ansiRed, s)
	}
	if side == 0 {
		switch c {
		case 'M', 'A', 'R', 'C', 'T':
			return paint(ansiGreen, s)
		case 'D':
			return paint(ansiRed, s)
		case 'U':
			return paint(ansiYellow, s)
		}
	} else {
		switch c {
		case 'M', 'D', 'T':
			return paint(ansiRed, s)
		case 'A':
			return paint(ansiGreen, s)
		case 'U':
			return paint(ansiYellow, s)
		}
	}
	return s
}

// colorCode renders a 2-char status code with per-char coloring. Synthetic
// rename markers `R<` (source) and `R>` (dest) get whole-code colors.
func colorCode(code string) string {
	if code == "R<" {
		return paint(ansiRed, code)
	}
	if code == "R>" {
		return paint(ansiGreen, code)
	}
	if len(code) != 2 {
		return code
	}
	return colorCodeChar(code[0], 0) + colorCodeChar(code[1], 1)
}

func formatLeaf(e *changeEntry, name string) string {
	out := dim("[") + colorCode(e.code) + dim("]") + " " + name
	if e.numstat != nil {
		out += " " + formatNumstat(*e.numstat)
	}
	return out
}

func formatNumstat(n numstat) string {
	return dim("(") +
		paint(ansiGreen, fmt.Sprintf("+%d", n.added)) +
		dim(",") +
		paint(ansiRed, fmt.Sprintf("-%d", n.deleted)) +
		dim(")")
}

func renderTree(root *treeNode, w io.Writer) {
	for _, k := range sortedChildren(root) {
		n := root.children[k]
		if n.entry == nil {
			fmt.Fprintf(w, "%s/\n", n.name)
		} else {
			fmt.Fprintln(w, formatLeaf(n.entry, n.name))
		}
		sub := sortedChildren(n)
		for i, sk := range sub {
			renderNode(w, n.children[sk], "", i == len(sub)-1)
		}
	}
}

func renderNode(w io.Writer, n *treeNode, prefix string, last bool) {
	branch := "├─ "
	nextPad := "│  "
	if last {
		branch = "└─ "
		nextPad = "   "
	}
	if n.entry == nil {
		fmt.Fprintf(w, "%s%s%s/\n", prefix, dim(branch), n.name)
	} else {
		fmt.Fprintf(w, "%s%s%s\n", prefix, dim(branch), formatLeaf(n.entry, n.name))
	}
	keys := sortedChildren(n)
	for i, k := range keys {
		renderNode(w, n.children[k], prefix+dim(nextPad), i == len(keys)-1)
	}
}

func sortedChildren(n *treeNode) []string {
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
