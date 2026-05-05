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

	// Build slot grid: each col occupies 2 chars (glyph + trailing space).
	// Diagonals (`/`, `\`) slide left into the previous col's trailing space
	// to render adjacent to a `|`, matching `git log --graph` output.
	slots := make([]Glyph, 2*numCols)
	for i := range slots {
		slots[i] = GlyphSpace
	}
	for c := 0; c < numCols; c++ {
		g := row.Glyphs[c]
		if g == GlyphSpace {
			continue
		}
		pos := 2 * c
		if (g == GlyphSlash || g == GlyphBackslash) && c > 0 {
			pos = 2*c - 1
		}
		slots[pos] = g
	}

	if row.Commit == nil {
		// Stagger / continuation row: emit slots verbatim including trailing
		// padding (matches git's fixed-width stagger lines).
		for _, g := range slots {
			b.WriteString(cs(KindGraph, g.String()))
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
	} else {
		b.WriteByte(' ')
		b.WriteString(cs(KindSubject, rest))
	}

	return b.String()
}
