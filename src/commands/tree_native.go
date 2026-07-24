package commands

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lczyk/gitgum/src/graph"
)

func (t *TreeCommand) renderNative(w io.Writer, sinceArg string, maxCount int) error {
	r := t.repo()

	// Build git log args: plumbing format with null-delimited segments.
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	// %ct (committer date) not %at (author date): git's --date-order sorts by
	// committer date, and the layout engine's row ordering must match it or an
	// open (never-merged) side branch whose author date predates the trunk tip
	// gets sorted to the bottom, detached from its fork point.
	gitArgs := []string{"log", "--all", logFormat, "--date-order", colorFlag}
	if sinceArg != "" {
		gitArgs = append(gitArgs, "--since", sinceArg)
	}
	if maxCount > 0 {
		gitArgs = append(gitArgs, fmt.Sprintf("-%d", maxCount))
	}

	stdout, _, runErr := r.Run(gitArgs...)
	if runErr != nil {
		return fmt.Errorf("git log: %w", runErr)
	}

	// --since is a global commit-date filter, so it can drop HEAD itself (and
	// the commits linking it to its in-window descendants) when the checked-out
	// commit predates the window (e.g. detached onto an old commit). That leaves
	// head-float with nothing to anchor on. Splice those lines back in so HEAD
	// stays in the graph, connected, and can sink to the bottom.
	if !t.NoHeadFloat {
		if extra := t.headFloatLines(colorFlag, stdout, nodeIDs(stdout)); len(extra) > 0 {
			stdout = strings.Join(extra, "\n") + "\n" + stdout
		}
	}

	if strings.TrimSpace(stdout) == "" {
		return nil
	}

	useColor := colorEnabled()
	nodes, err := parseNativeCommits(stdout, useColor, !t.NoHeadFloat)
	if err != nil {
		return fmt.Errorf("parsing git log output: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	// Reverse is a layout concern, not a text one: the graph knows which
	// characters are edges, a pass over rendered lines would have to guess.
	lr := graph.Layout(nodes, graph.Opt{Reverse: t.Reverse})

	st := graph.Style{}
	if useColor {
		st = graph.Style{LinePrefix: ansiRed, LineSuffix: ansiReset}
	}

	for _, line := range graph.Render(lr, st) {
		fmt.Fprintln(w, line)
	}
	return nil
}

// logFormat is the null-delimited plumbing format shared by renderNative's main
// query and the head-float splice, so spliced lines parse identically.
const logFormat = "--format=%H %P%x00%h%d %s%x00%ct"

// rawHasNode reports whether id appears as a node (the leading %H field of some
// line) in git-log output, ignoring matches in the %P parent fields.
func rawHasNode(raw, id string) bool {
	prefix := id + " "
	for _, ln := range strings.Split(raw, "\n") {
		if strings.HasPrefix(ln, prefix) {
			return true
		}
	}
	return false
}

// nodeIDs extracts the %H node id (leading field) from each line of git-log
// output, ignoring blanks.
func nodeIDs(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	ids := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		if id, _, ok := strings.Cut(ln, " "); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// headFloatLines returns extra git-log lines to splice into the main output so
// the checked-out commit (HEAD) and the commits bridging it to its in-window
// descendants stay visible -- --since can filter HEAD and that connecting chain
// out. windowIDs are the %H node ids already present in raw. Returns nil when
// HEAD is already shown (the common in-window case) or on lookup failure.
// colorFlag mirrors the main query so spliced lines format identically.
func (t *TreeCommand) headFloatLines(colorFlag, raw string, windowIDs []string) []string {
	r := t.repo()
	idOut, _, err := r.Run("rev-parse", "HEAD")
	if err != nil {
		return nil
	}
	headID := strings.TrimSpace(idOut)
	// Match HEAD as a node id (%H, line-start), not anywhere: the hash also
	// appears as a %P parent field on HEAD's children, so a plain substring
	// check would wrongly treat an out-of-window HEAD as already present.
	if headID == "" || rawHasNode(raw, headID) {
		return nil
	}

	var lines []string
	// HEAD itself: the A..B bridge ranges below exclude A, so they never yield
	// HEAD -- fetch it as its own single-rev line.
	if out, _, err := r.Run("log", "-1", logFormat, colorFlag, headID); err == nil {
		if line := strings.TrimRight(out, "\n"); line != "" {
			lines = append(lines, line)
		}
	}

	// Bridge: commits on the ancestry path from HEAD (exclusive) up to each
	// in-window node. --ancestry-path trims each HEAD..id range to the commits
	// actually linking the two; git dedups across ranges in a single walk.
	// Splice only the ones --since dropped (not already in raw).
	if len(windowIDs) > 0 {
		args := []string{"log", "--ancestry-path", logFormat, colorFlag}
		for _, id := range windowIDs {
			args = append(args, headID+".."+id)
		}
		if out, _, err := r.Run(args...); err == nil {
			for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if ln == "" {
					continue
				}
				if id, _, ok := strings.Cut(ln, " "); ok && !rawHasNode(raw, id) {
					lines = append(lines, ln)
				}
			}
		}
	}
	return lines
}

// parseNativeCommits parses null-delimited git log output and pre-formats
// each Label with ANSI escapes when color is on. Each commit is one line:
// "<hash> <parents>\x00<hash> <decorations> <subject>\x00<epoch>"
//
// floatHead controls whether the checked-out commit (HEAD decoration) gets
// Node.Float set, sinking it and its descendants to the bottom of the layout.
func parseNativeCommits(raw string, useColor, floatHead bool) ([]graph.Node, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	lines := strings.Split(raw, "\n")
	nodes := make([]graph.Node, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		seg := strings.SplitN(line, "\x00", 3)
		if len(seg) < 2 {
			continue
		}
		topo := strings.Fields(seg[0])
		if len(topo) == 0 {
			continue
		}
		id := topo[0]
		var parents []string
		if len(topo) > 1 {
			parents = topo[1:]
		}
		// Strip trailing whitespace -- empty %s leaves a trailing space
		// after the hash that the old per-segment render dropped.
		rawLabel := strings.TrimRight(seg[1], " ")
		var epoch int64
		if len(seg) > 2 {
			epoch, _ = strconv.ParseInt(strings.TrimSpace(seg[2]), 10, 64)
		}
		label := rawLabel
		if useColor {
			label = colorLabel(rawLabel)
		}
		nodes = append(nodes, graph.Node{
			ID:      id,
			Label:   label,
			Parents: parents,
			Epoch:   epoch,
			Float:   floatHead && isHeadDecoration(rawLabel),
		})
	}
	return nodes, nil
}

// isHeadDecoration reports whether the %d decoration marks the checked-out
// commit. Matches both attached ("HEAD -> main") and detached ("HEAD") forms,
// scoped to a ref token so a subject mentioning the word HEAD never trips it.
func isHeadDecoration(label string) bool {
	idx := strings.Index(label, " (")
	if idx < 0 {
		return false
	}
	rest := label[idx+2:]
	end := strings.Index(rest, ")")
	if end < 0 {
		return false
	}
	for _, ref := range strings.Split(rest[:end], ", ") {
		ref = strings.TrimSpace(ref)
		if ref == "HEAD" || strings.HasPrefix(ref, "HEAD -> ") {
			return true
		}
	}
	return false
}

// colorLabel takes a raw "<hash> [(refs)] <subject>" string and returns
// it with ANSI escapes baked in: hash yellow, refs decorated per git's
// color.decorate defaults, subject plain.
func colorLabel(label string) string {
	hashEnd := strings.IndexByte(label, ' ')
	if hashEnd < 0 {
		return ansiYellow + label + ansiReset
	}
	hash := label[:hashEnd]
	rest := label[hashEnd+1:]

	var b strings.Builder
	b.WriteString(ansiYellow)
	b.WriteString(hash)
	b.WriteString(ansiReset)

	if len(rest) > 0 && rest[0] == '(' {
		if refEnd := strings.IndexByte(rest, ')'); refEnd >= 0 {
			refs := rest[:refEnd+1]
			subject := strings.TrimLeft(rest[refEnd+1:], " ")
			b.WriteByte(' ')
			b.WriteString(colorRefDecoration(refs))
			if subject != "" {
				b.WriteByte(' ')
				b.WriteString(colorCommitSubject(subject, extractTags(refs)))
			}
			return b.String()
		}
	}
	if rest != "" {
		b.WriteByte(' ')
		b.WriteString(colorCommitSubject(rest, nil))
	}
	return b.String()
}

// colorRefDecoration colors a "(refs...)" string per git's color.decorate
// defaults: HEAD = bold cyan, local branch = bold green, remote branch =
// bold red, tag = bold yellow, parens/separators = bold yellow.
func colorRefDecoration(text string) string {
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return text
	}
	inner := text[1 : len(text)-1]

	var b strings.Builder
	b.WriteString(ansiBoldYellow)
	b.WriteByte('(')
	b.WriteString(ansiReset)

	parts := strings.Split(inner, ", ")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(ansiBoldYellow)
			b.WriteString(", ")
			b.WriteString(ansiReset)
		}
		if arrow := strings.Index(p, " -> "); arrow >= 0 {
			head := p[:arrow]
			branch := p[arrow+4:]
			b.WriteString(ansiBoldCyan)
			b.WriteString(head)
			b.WriteString(ansiReset)
			b.WriteString(ansiBoldYellow)
			b.WriteString(" -> ")
			b.WriteString(ansiReset)
			b.WriteString(colorSingleRef(branch))
		} else {
			b.WriteString(colorSingleRef(p))
		}
	}

	b.WriteString(ansiBoldYellow)
	b.WriteByte(')')
	b.WriteString(ansiReset)
	return b.String()
}

func colorSingleRef(r string) string {
	switch {
	case strings.HasPrefix(r, "tag: "):
		return ansiBoldYellow + r + ansiReset
	case r == "HEAD":
		return ansiBoldCyan + r + ansiReset
	case strings.Contains(r, "/"):
		return ansiBoldRed + r + ansiReset
	default:
		return ansiBoldGreen + r + ansiReset
	}
}

const (
	ansiBoldBlue    = "\033[1;34m"
	ansiBoldCyan    = "\033[1;36m"
	ansiBoldGreen   = "\033[1;32m"
	ansiBoldMagenta = "\033[1;35m"
	ansiBoldPink    = "\033[1;38;5;205m"
	ansiBoldPurple  = "\033[1;38;5;97m"
	ansiBoldRed     = "\033[1;31m"
	ansiBoldYellow  = "\033[1;33m"
)
