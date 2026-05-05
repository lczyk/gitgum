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

	firstNonSpace := -1
	lastNonSpace := -1
	for c := 0; c < numCols; c++ {
		if row.Glyphs[c] != GlyphSpace {
			if firstNonSpace == -1 {
				firstNonSpace = c
			}
			lastNonSpace = c
		}
	}

	if firstNonSpace == -1 {
		return ""
	}

	for c := firstNonSpace; c <= lastNonSpace; c++ {
		b.WriteString(cs(KindGraph, row.Glyphs[c].String()))
	}

	if row.Commit == nil {
		return b.String()
	}

	// Append label after a space.
	if b.Len() > 0 {
		b.WriteByte(' ')
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
