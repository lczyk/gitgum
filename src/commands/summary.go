package commands

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// defaultSummaryWidth is what the layout assumes when there is no terminal to
// measure -- output being piped, or a test buffer.
const defaultSummaryWidth = 80

// maxSummaryName caps the path column, matching git's own default. Without a
// cap a repo with deep paths spends the whole terminal on names and leaves no
// room for the bars, which are the reason to draw this rather than list files.
const maxSummaryName = 50

// minSummaryTail is what the row needs after the name -- the separators, the
// count column and a bar worth looking at. A terminal too narrow for both
// gives the name back the space rather than the bar.
const minSummaryTail = 15

// diffSummary renders a diff file by file: the path, how many lines moved,
// and a bar in proportion, then a totals line. It is the shared diffstat used
// by diff, pull, push and clone, so it lives here rather than inside any one
// command's file.
//
// gg draws this rather than printing git's --compact-summary because git sizes
// that layout to *its own* stdout, which is a pipe whenever gg is reading it --
// so the bars were always drawn for an 80-column terminal whatever the user
// actually had. width is the terminal to lay out for; zero measures stdout.
//
// extra is passed to git verbatim: "--cached", a revision range, pathspecs.
func diffSummary(r git.Repo, width int, extra ...string) (string, error) {
	files, err := r.DiffFiles(extra...)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	return renderDiffSummary(files, width, colorEnabled()), nil
}

// summaryRow is one file, reduced to what the layout places: a name (a rename
// already collapsed into one), the count column, and the bar.
type summaryRow struct {
	name    string
	total   int
	added   int
	deleted int
	binary  bool
}

func renderDiffSummary(files []git.DiffFile, width int, color bool) string {
	if width <= 0 {
		width = stdoutWidth()
	}
	if width <= 0 {
		width = defaultSummaryWidth
	}

	rows := make([]summaryRow, 0, len(files))
	nameWidth, countWidth, maxTotal, added, deleted := 0, 0, 0, 0, 0
	for _, f := range files {
		row := summaryRow{
			name:    summaryName(f),
			total:   f.Added + f.Deleted,
			added:   f.Added,
			deleted: f.Deleted,
			binary:  f.Binary,
		}
		rows = append(rows, row)
		nameWidth = max(nameWidth, len(row.name))
		countWidth = max(countWidth, len(fmt.Sprint(row.total)))
		maxTotal = max(maxTotal, row.total)
		added += f.Added
		deleted += f.Deleted
	}

	nameWidth = min(nameWidth, max(min(maxSummaryName, width-minSummaryTail), 10))

	// " name | count bar": the constant is the leading space, the " | "
	// separator and the space before the bar.
	barWidth := width - nameWidth - countWidth - 5

	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, " %-*s | ", nameWidth, elidePath(row.name, nameWidth))
		if row.binary {
			b.WriteString("Bin\n")
			continue
		}
		fmt.Fprintf(&b, "%*d", countWidth, row.total)
		if bar := summaryBar(row, maxTotal, barWidth, color); bar != "" {
			b.WriteString(" " + bar)
		}
		b.WriteByte('\n')
	}
	b.WriteString(" " + summaryTotals(len(rows), added, deleted))
	return b.String()
}

// summaryName is the path as the row shows it, a rename collapsed into one
// entry and any mode change noted after it.
func summaryName(f git.DiffFile) string {
	name := f.Path
	if f.Renamed() {
		name = renamePath(f.RenamedFrom, f.Path)
	}
	if note := modeNote(f); note != "" {
		name += " " + note
	}
	return name
}

// elidePath shortens a path from the front to fit the column, marking the cut
// with "...". What was dropped is the part a reader scanning a list of files
// least needs -- the leading directories are what the rows have in common.
//
// The cut lands on a separator when one falls inside what is kept, so the
// remainder starts at a directory rather than mid-name.
func elidePath(p string, width int) string {
	const ellipsis = "..."
	if len(p) <= width || width <= len(ellipsis) {
		return p
	}
	tail := p[len(p)-(width-len(ellipsis)):]
	if i := strings.IndexByte(tail, '/'); i >= 0 {
		tail = tail[i:]
	}
	return ellipsis + tail
}

// renamePath writes a rename the way git does, with the parts both paths share
// left outside the braces: "src/{alpha => beta}/file.go". Only whole path
// segments are shared, so a rename within a directory stays readable.
func renamePath(from, to string) string {
	prefix := commonSegments(from, to)
	fromRest, toRest := from[len(prefix):], to[len(prefix):]

	suffix := commonSegments(reverseSegments(fromRest), reverseSegments(toRest))
	suffix = reverseSegments(suffix)
	fromRest = fromRest[:len(fromRest)-len(suffix)]
	toRest = toRest[:len(toRest)-len(suffix)]

	if prefix == "" && suffix == "" {
		return from + " => " + to
	}
	return prefix + "{" + fromRest + " => " + toRest + "}" + suffix
}

// commonSegments is the longest shared prefix of a and b that ends on a '/',
// so "src/alpha/x" and "src/alto/y" share "src/" rather than "src/al".
func commonSegments(a, b string) string {
	cut := 0
	for i := 0; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
		if a[i] == '/' {
			cut = i + 1
		}
	}
	return a[:cut]
}

// reverseSegments flips a path segment-wise ("a/b/c" -> "c/b/a"), so the same
// prefix search finds a shared suffix.
func reverseSegments(p string) string {
	parts := strings.Split(p, "/")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// modeNote names what happened to the file itself rather than its contents:
// it appeared, it went away, or its mode changed. Empty for an ordinary edit.
func modeNote(f git.DiffFile) string {
	var parts []string
	switch {
	case f.NewMode == git.ModeAbsent:
		return "(gone)"
	case f.OldMode == git.ModeAbsent:
		parts = append(parts, "new")
	case f.OldMode != f.NewMode:
		parts = append(parts, "mode")
	default:
		return ""
	}
	if bit := modeBit(f.OldMode, f.NewMode); bit != "" {
		parts = append(parts, bit)
	}
	return "(" + strings.Join(parts, " ") + ")"
}

// modeBit names the bit that moved: the file became or stopped being a
// symlink, or gained or lost the executable bit.
func modeBit(old, new string) string {
	switch {
	case new == git.ModeSymlink && old != git.ModeSymlink:
		return "+l"
	case old == git.ModeSymlink && new != git.ModeSymlink:
		return "-l"
	case new == git.ModeExec && old != git.ModeExec:
		return "+x"
	case old == git.ModeExec && new != git.ModeExec:
		return "-x"
	}
	return ""
}

// summaryBar draws the change in proportion to the largest change in the same
// diff, so the rows are comparable to each other rather than each to itself.
// A file that changed at all keeps at least one cell: rounding a real change
// away would read as no change.
func summaryBar(row summaryRow, maxTotal, barWidth int, color bool) string {
	if row.total == 0 || barWidth <= 0 {
		return ""
	}
	cells := row.total
	if maxTotal > barWidth {
		cells = max(scale(row.total, barWidth, maxTotal), 1)
	}
	plus, minus := split(row.added, row.deleted, cells)
	if color {
		return paint(ansiGreen, strings.Repeat("+", plus)) + paint(ansiRed, strings.Repeat("-", minus))
	}
	return strings.Repeat("+", plus) + strings.Repeat("-", minus)
}

// split shares the cells between insertions and deletions, giving a side that
// changed anything at least one cell to say so with.
func split(added, deleted, cells int) (plus, minus int) {
	switch {
	case deleted == 0:
		return cells, 0
	case added == 0:
		return 0, cells
	}
	plus = max(scale(added, cells, added+deleted), 1)
	minus = max(cells-plus, 1)
	if plus+minus > cells && plus > 1 {
		plus = cells - minus
	}
	return plus, minus
}

// scale is n*of/total rounded to nearest rather than down. Truncating loses
// most of a cell on every row, which over a whole diff reads as less change
// than there was.
func scale(n, of, total int) int {
	if total == 0 {
		return 0
	}
	return (n*of + total/2) / total
}

// summaryTotals is git's closing line, down to which halves it prints: a zero
// count is dropped unless both are zero, when saying nothing changed twice is
// clearer than saying nothing at all.
func summaryTotals(files, added, deleted int) string {
	parts := []string{fmt.Sprintf("%d %s changed", files, plural(files, "file", "files"))}
	if added > 0 || deleted == 0 {
		parts = append(parts, fmt.Sprintf("%d %s(+)", added, plural(added, "insertion", "insertions")))
	}
	if deleted > 0 || added == 0 {
		parts = append(parts, fmt.Sprintf("%d %s(-)", deleted, plural(deleted, "deletion", "deletions")))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
