package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type changeEntry struct {
	code string // 2-char porcelain XY, or "R<"/"R>" for rename source/dest
	path string
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

// TODO: respect NO_COLOR / non-tty -- currently ansi escapes always emitted.
const (
	ansiReset  = "\033[0m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

func dim(s string) string { return ansiDim + s + ansiReset }

// colorCodeChar colors one porcelain status char. side==0 is X (staged),
// side==1 is Y (worktree). Rules mirror git's own status colors.
func colorCodeChar(c byte, side int) string {
	s := string(c)
	switch c {
	case ' ':
		return dim(s)
	case '?', '!':
		return ansiRed + s + ansiReset
	}
	if side == 0 {
		switch c {
		case 'M', 'A', 'R', 'C', 'T':
			return ansiGreen + s + ansiReset
		case 'D':
			return ansiRed + s + ansiReset
		case 'U':
			return ansiYellow + s + ansiReset
		}
	} else {
		switch c {
		case 'M', 'D', 'T':
			return ansiRed + s + ansiReset
		case 'A':
			return ansiGreen + s + ansiReset
		case 'U':
			return ansiYellow + s + ansiReset
		}
	}
	return s
}

// colorCode renders a 2-char status code with per-char coloring. Synthetic
// rename markers `R<` (source) and `R>` (dest) get whole-code colors.
func colorCode(code string) string {
	if code == "R<" {
		return ansiRed + code + ansiReset
	}
	if code == "R>" {
		return ansiGreen + code + ansiReset
	}
	if len(code) != 2 {
		return code
	}
	return colorCodeChar(code[0], 0) + colorCodeChar(code[1], 1)
}

func formatLeaf(e *changeEntry, name string) string {
	return dim("[") + colorCode(e.code) + dim("]") + " " + name
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
