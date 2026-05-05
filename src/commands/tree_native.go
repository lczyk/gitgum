package commands

import (
	"fmt"
	"io"
	"os"
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
	gitArgs := []string{"log", "--all", "--format=%H %P%x00%h%d %s%x00%at", "--date-order", colorFlag}
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
	if strings.TrimSpace(stdout) == "" {
		return nil
	}

	useColor := nativeColorEnabled()
	nodes, err := parseNativeCommits(stdout, useColor)
	if err != nil {
		return fmt.Errorf("parsing git log output: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	lr := graph.Layout(nodes)

	if os.Getenv("GG_DUMP_LAYOUT") == "1" {
		fmt.Fprintf(os.Stderr, "columns=%d rows=%d\n", lr.Columns, len(lr.Rows))
		for i, row := range lr.Rows {
			if i >= 30 {
				break
			}
			var gs string
			for _, g := range row.Glyphs {
				gs += g.String()
			}
			label := ""
			if row.Commit != nil {
				label = row.Commit.Label
				if len(label) > 50 {
					label = label[:50]
				}
			}
			fmt.Fprintf(os.Stderr, "  row %2d: %-8s %s\n", i, gs, label)
		}
	}

	st := graph.Style{}
	if useColor {
		st = graph.Style{LinePrefix: ansiRed, LineSuffix: ansiReset}
	}

	for _, line := range graph.Render(lr, st) {
		fmt.Fprintln(w, line)
	}
	return nil
}

// nativeColorEnabled mirrors the prior nativeColorScheme gating: color
// is off only when stdout isn't a tty, FORCE_COLOR is unset, and
// NO_COLOR is set. Otherwise color is on.
func nativeColorEnabled() bool {
	_, fc := os.LookupEnv("FORCE_COLOR")
	_, nc := os.LookupEnv("NO_COLOR")
	return colorEnabled() || fc || !nc
}

// parseNativeCommits parses null-delimited git log output and pre-formats
// each Label with ANSI escapes when color is on. Each commit is one line:
// "<hash> <parents>\x00<hash> <decorations> <subject>\x00<date>"
func parseNativeCommits(raw string, useColor bool) ([]graph.Node, error) {
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
		hint := extractLayoutHint(rawLabel)
		label := rawLabel
		if useColor {
			label = colorLabel(rawLabel)
		}
		nodes = append(nodes, graph.Node{
			ID:         id,
			Label:      label,
			Parents:    parents,
			Epoch:      epoch,
			LayoutHint: hint,
		})
	}
	return nodes, nil
}

// extractLayoutHint parses the first branch name from git's %d decoration
// string. Format: "abc1234 (HEAD -> main, origin/main) subject" -> "main".
// Returns "" if no ref decoration is present.
func extractLayoutHint(label string) string {
	if idx := strings.Index(label, " ("); idx >= 0 {
		rest := label[idx+2:]
		if end := strings.Index(rest, ")"); end >= 0 {
			refs := rest[:end]
			refs = strings.TrimPrefix(refs, "HEAD -> ")
			if comma := strings.Index(refs, ","); comma >= 0 {
				refs = refs[:comma]
			}
			refs = strings.TrimSpace(refs)
			if slash := strings.Index(refs, "/"); slash >= 0 && slash < len(refs)-1 {
				refs = refs[slash+1:]
			}
			if refs != "" && !strings.HasPrefix(refs, "tag: ") {
				return refs
			}
		}
	}
	return ""
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
				b.WriteString(subject)
			}
			return b.String()
		}
	}
	if rest != "" {
		b.WriteByte(' ')
		b.WriteString(rest)
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
	ansiBoldCyan   = "\033[1;36m"
	ansiBoldGreen  = "\033[1;32m"
	ansiBoldRed    = "\033[1;31m"
	ansiBoldYellow = "\033[1;33m"
)
