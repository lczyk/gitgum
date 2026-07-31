package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/src/filetree"
)

type numstat struct {
	added, deleted int
	unknown        bool // line count not available (e.g. timed-out untracked read)
	binary         bool // file detected as binary
}

// numstats reads `git diff --numstat HEAD --no-renames` into a path-keyed map.
// Untracked files, binary diffs, rename markers and empty repos (no HEAD) have
// no entry -- the renderer simply omits the count for those.
func numstats(repo git.Repo) map[string]numstat {
	out, _, err := repo.Run("diff", "--numstat", "--no-renames", "HEAD")
	if err != nil {
		return nil
	}
	return parseNumstat(out)
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
	ansiPurple  = "\033[38;5;97m"
	ansiPink    = "\033[38;5;205m"
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

// leafItem is one path decorated the way gg draws changes: the status code in
// brackets on the left, the diffstat on the right, the name left to the
// renderer since folding and quoting are both its to decide.
func leafItem(code, path string, ns *numstat) filetree.Item {
	item := filetree.Item{
		Path:  path,
		IsDir: strings.HasSuffix(path, "/"),
	}
	if code != "" {
		item.Prefix = dim("[") + colorCode(code) + dim("]") + " "
	}
	if ns != nil {
		item.Suffix = " " + formatNumstat(*ns)
	}
	return item
}

// treeOpts is how gg draws a working-tree listing: chains folded, box drawing
// dimmed, height bounded. Shared so the three commands that show one cannot
// drift apart in how it looks.
func treeOpts(maxLines int) filetree.Opts {
	return filetree.Opts{Dim: dim, FoldChains: true, MaxLines: maxLines}
}

// statusItems decorates a scan. A rename becomes two entries, source and
// destination.
func statusItems(entries []git.Entry, stats map[string]numstat) []filetree.Item {
	var out []filetree.Item
	add := func(code, path string) {
		var ns *numstat
		if n, ok := stats[strings.TrimSuffix(path, "/")]; ok {
			ns = &n
		}
		out = append(out, leafItem(code, path, ns))
	}
	for _, e := range entries {
		if e.RenamedFrom != "" {
			add("R<", e.RenamedFrom)
			add("R>", e.Path)
			continue
		}
		add(string(e.X)+string(e.Y), e.Path)
	}
	return out
}

// flatItems reproduces git's own listing: the raw two-character code, then the
// path, with a rename written as "old -> new" on one line. Uncoloured, since
// what --flat offers is git's shape rather than gg's.
func flatItems(entries []git.Entry) []filetree.Item {
	out := make([]filetree.Item, 0, len(entries))
	for _, e := range entries {
		prefix := string(e.X) + string(e.Y) + " "
		if e.RenamedFrom != "" {
			prefix += filetree.QuoteName(e.RenamedFrom) + " -> "
		}
		out = append(out, filetree.Item{
			Path:   e.Path,
			IsDir:  strings.HasSuffix(e.Path, "/"),
			Prefix: prefix,
		})
	}
	return out
}

func formatNumstat(n numstat) string {
	if n.binary {
		return dim("(") + paint(ansiYellow, "Bin") + dim(")")
	}
	if n.unknown {
		return dim("(") +
			paint(ansiGreen, "+???") +
			dim(",") +
			paint(ansiRed, "-???") +
			dim(") ") +
			paint(ansiGreen, "+++") +
			paint(ansiRed, "---")
	}
	return dim("(") +
		paint(ansiGreen, fmt.Sprintf("+%d", n.added)) +
		dim(",") +
		paint(ansiRed, fmt.Sprintf("-%d", n.deleted)) +
		dim(")")
}
