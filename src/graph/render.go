package graph

import "strings"

// Render produces output lines in `git log --graph --oneline` style.
// If cs is nil, output is plain ASCII.
func Render(lr LayoutResult, cs ColorScheme) []string {
	if cs == nil {
		cs = func(k GlyphKind, text string) string { return text }
	}

	var lines []string
	for _, row := range lr.Rows {
		line := renderRow(row, lr.Columns, cs)
		lines = append(lines, line)
	}
	return lines
}

func renderRow(row Row, numCols int, cs ColorScheme) string {
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

	if row.Commit == nil {
		// Stagger / continuation row: render up to the rightmost col with
		// non-space content (primary or slid diagonal), full 2-char slot
		// width. Matches git's stagger-row trim-to-active behavior.
		lastCol := -1
		for c := 0; c < numCols; c++ {
			if row.Glyphs[c] != GlyphSpace {
				lastCol = c
			}
		}
		if lastCol < 0 {
			return ""
		}
		for i := 0; i < 2*(lastCol+1); i++ {
			b.WriteString(cs(KindGraph, slots[i].String()))
		}
		return b.String()
	}

	// Commit row: render up to rightmost active col only; label follows
	// immediately after that col's trailing space.
	lastActive := -1
	for c := 0; c < numCols; c++ {
		if row.Glyphs[c] != GlyphSpace {
			lastActive = c
		}
	}
	if lastActive < 0 {
		lastActive = 0
	}
	for i := 0; i < 2*(lastActive+1); i++ {
		b.WriteString(cs(KindGraph, slots[i].String()))
	}

	// Merge commits get (n-1)*2 extra space chars after the `*` slot to
	// align with the stagger row above. Matches git's --graph output.
	if extra := len(row.Commit.Parents) - 1; extra > 0 {
		for i := 0; i < extra*2; i++ {
			b.WriteString(cs(KindGraph, " "))
		}
	}

	// Split label into hash, refs, and subject parts for coloring.
	// Expected format: "<hash> <refs> <subject>" or "<hash> <subject>"
	// where refs are wrapped in "(...)" if present.
	label := row.Commit.Label
	hashEnd := strings.IndexByte(label, ' ')
	if hashEnd < 0 {
		b.WriteString(cs(KindHash, label))
		return b.String()
	}
	hash := label[:hashEnd]
	rest := label[hashEnd+1:]

	b.WriteString(cs(KindHash, hash))

	if len(rest) > 0 && rest[0] == '(' {
		// There's a ref decoration.
		refEnd := strings.IndexByte(rest, ')')
		if refEnd >= 0 {
			refs := rest[:refEnd+1]
			subject := strings.TrimLeft(rest[refEnd+1:], " ")
			b.WriteByte(' ')
			b.WriteString(cs(KindRef, refs))
			if subject != "" {
				b.WriteByte(' ')
				b.WriteString(cs(KindSubject, subject))
			}
		} else {
			b.WriteByte(' ')
			b.WriteString(cs(KindSubject, rest))
		}
	} else if rest != "" {
		b.WriteByte(' ')
		b.WriteString(cs(KindSubject, rest))
	}

	return b.String()
}
