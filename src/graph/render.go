package graph

import "strings"

// Render produces output lines in `git log --graph --oneline` style.
// Style controls graph-glyph ANSI wrapping; pass the zero Style for plain
// ASCII. Labels are appended verbatim -- callers wanting per-segment
// coloring should embed ANSI escapes in Node.Label before calling Layout.
func Render(lr LayoutResult, st Style) []string {
	lines := make([]string, 0, len(lr.Rows))
	for _, row := range lr.Rows {
		lines = append(lines, renderRow(row, lr.Columns, st))
	}
	return lines
}

func renderRow(row Row, numCols int, st Style) string {
	var b strings.Builder

	// Build slot grid by packing left-to-right. Diagonals (`/`, `\`) slide
	// into the previous col's trailing-space slot, and the next col's
	// primary slides up too -- this is git's compressed `|\|` cross-routing
	// pattern. Without packing, multi-col layouts get extra whitespace.
	slots := make([]Glyph, 0, 2*numCols)
	for c := 0; c < numCols; c++ {
		g := row.Glyphs[c]
		if (g == GlyphSlash || g == GlyphBackslash) && len(slots) > 0 && slots[len(slots)-1] == GlyphSpace {
			slots[len(slots)-1] = g
			continue
		}
		if g == GlyphSpace {
			slots = append(slots, GlyphSpace, GlyphSpace)
		} else {
			slots = append(slots, g, GlyphSpace)
		}
	}
	for len(slots) < 2*numCols {
		slots = append(slots, GlyphSpace)
	}

	// Determine right edge: stagger rows render up to the rightmost col with
	// non-space content; commit rows do the same (with at least col 0).
	lastActive := -1
	for c := 0; c < numCols; c++ {
		if row.Glyphs[c] != GlyphSpace {
			lastActive = c
		}
	}
	if row.Commit == nil {
		if lastActive < 0 {
			return ""
		}
	} else if lastActive < 0 {
		lastActive = 0
	}

	writeSlots(&b, slots[:2*(lastActive+1)], st)

	if row.Commit == nil {
		return b.String()
	}

	// Merge commits get (n-1)*2 extra space chars after the `*` slot to
	// align with the stagger row above. Matches git's --graph output.
	if extra := len(row.Commit.Parents) - 1; extra > 0 {
		for range extra * 2 {
			b.WriteByte(' ')
		}
	}

	// Label is opaque -- callers embed ANSI codes pre-Layout if they want
	// per-segment coloring.
	b.WriteString(row.Commit.Label)
	return b.String()
}

// writeSlots emits glyph runs of identical Glyph as a single styled
// write. Lines (`|`/`/`/`\`) are wrapped with Style.LinePrefix/LineSuffix,
// stars with Style.StarPrefix/StarSuffix; spaces and unstyled cases go
// straight to the builder.
func writeSlots(b *strings.Builder, slots []Glyph, st Style) {
	if len(slots) == 0 {
		return
	}
	runStart := 0
	for i := 1; i <= len(slots); i++ {
		if i < len(slots) && slots[i] == slots[runStart] {
			continue
		}
		g := slots[runStart]
		n := i - runStart
		ch := g.String()
		switch g {
		case GlyphSpace:
			for range n {
				b.WriteByte(' ')
			}
		case GlyphStar:
			if st.StarPrefix == "" && st.StarSuffix == "" {
				for range n {
					b.WriteString(ch)
				}
			} else {
				b.WriteString(st.StarPrefix)
				for range n {
					b.WriteString(ch)
				}
				b.WriteString(st.StarSuffix)
			}
		default: // pipe, slash, backslash
			if st.LinePrefix == "" && st.LineSuffix == "" {
				for range n {
					b.WriteString(ch)
				}
			} else {
				b.WriteString(st.LinePrefix)
				for range n {
					b.WriteString(ch)
				}
				b.WriteString(st.LineSuffix)
			}
		}
		runStart = i
	}
}
