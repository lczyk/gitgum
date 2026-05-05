package commands

import (
	"fmt"
	"io"
	"os"
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
	gitArgs := []string{"log", "--all", "--format=%H %P%x00%h%d %s%x00%aI", "--date-order", colorFlag}
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

	// Parse commit records into []graph.Node.
	nodes, err := parseNativeCommits(stdout)
	if err != nil {
		return fmt.Errorf("parsing git log output: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	// Layout and render.
	var eng graph.Engine
	lr := eng.Layout(graph.Graph{Nodes: nodes})

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

	cs := nativeColorScheme()
	lines := graph.Render(lr, cs)

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return nil
}

// parseNativeCommits parses null-delimited git log output. Each commit is
// one line containing: "<hash> <parents>\x00<hash> <decorations> <subject>\x00<date>"
func parseNativeCommits(raw string) ([]graph.Node, error) {
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
		// seg[0] = "<full-hash> <parent1> <parent2> ..."
		// seg[1] = "<abbrev-hash> <decorations> <subject>"
		// seg[2] = "<ISO-date>" (optional)
		topo := strings.Fields(seg[0])
		if len(topo) == 0 {
			continue
		}
		id := topo[0]
		var parents []string
		if len(topo) > 1 {
			parents = topo[1:]
		}
		label := seg[1]
		date := ""
		if len(seg) > 2 {
			date = strings.TrimSpace(seg[2])
		}
		hint := extractLayoutHint(label)
		nodes = append(nodes, graph.Node{
			ID:         id,
			Label:      label,
			Parents:    parents,
			Date:       date,
			LayoutHint: hint,
		})
	}
	return nodes, nil
}

// nativeColorScheme returns a ColorScheme that mirrors git's default colors.
// For now it returns nil (plain output). Colored output will be added when
// we match git's exact ANSI scheme.
// extractLayoutHint parses the first branch name from git's %d decoration
// string. Format: "abc1234 (HEAD -> main, origin/main) subject" → "main".
// Returns "" if no ref decoration is present.
func extractLayoutHint(label string) string {
	if idx := strings.Index(label, " ("); idx >= 0 {
		rest := label[idx+2:]
		if end := strings.Index(rest, ")"); end >= 0 {
			refs := rest[:end]
			// Skip "HEAD -> " if present.
			refs = strings.TrimPrefix(refs, "HEAD -> ")
			// Take the first ref (before comma).
			if comma := strings.Index(refs, ","); comma >= 0 {
				refs = refs[:comma]
			}
			refs = strings.TrimSpace(refs)
			// Strip remote prefix to get the branch name. "origin/main" → "main".
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

func nativeColorScheme() graph.ColorScheme {
	_, fc := os.LookupEnv("FORCE_COLOR")
	_, nc := os.LookupEnv("NO_COLOR")
	if !colorEnabled() && !fc && nc {
		return nil
	}
	// Placeholder: return a simple color scheme.
	return func(k graph.GlyphKind, text string) string {
		switch k {
		case graph.KindGraph:
			return ansiBlue + text + ansiReset
		case graph.KindHash:
			return ansiYellow + text + ansiReset
		case graph.KindRef:
			return ansiCyan + text + ansiReset
		case graph.KindSubject:
			return text
		}
		return text
	}
}

const (
	ansiBlue = "\033[34m"
	ansiCyan = "\033[36m"
)
