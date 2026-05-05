package graph

import "strings"

// Render produces output lines in `git log --graph --oneline` style.
// If cs is nil, output is plain ASCII and the per-fragment ColorScheme
// closure is bypassed entirely (saves a function call per glyph run).
func Render(lr LayoutResult, cs ColorScheme) []string {
	lines := make([]string, 0, len(lr.Rows))
	for _, row := range lr.Rows {
		lines = append(lines, renderRow(row, lr.Columns, cs))
	}
	return lines
}

// writeGraphSlots emits glyph runs of identical Glyph as a single cs()
// call. ColorSchemes that want to wrap a run of pipes in colour see
// "||||" instead of four separate "|" calls. With nil cs the runs go
// straight to the builder, skipping the closure call entirely.
func writeGraphSlots(b *strings.Builder, slots []Glyph, cs ColorScheme) {
	if len(slots) == 0 {
		return
	}
	runStart := 0
	for i := 1; i <= len(slots); i++ {
		if i < len(slots) && slots[i] == slots[runStart] {
			continue
		}
		ch := slots[runStart].String()
		n := i - runStart
		if cs == nil {
			for k := 0; k < n; k++ {
				b.WriteString(ch)
			}
		} else {
			run := ch
			if n > 1 {
				run = strings.Repeat(ch, n)
			}
			b.WriteString(cs(KindGraph, run))
		}
		runStart = i
	}
}

// csCall is the nil-aware cs() invocation for the non-graph kinds
// (Hash / Ref / Subject). Returns text unchanged when cs is nil.
func csCall(cs ColorScheme, kind GlyphKind, text string) string {
	if cs == nil {
		return text
	}
	return cs(kind, text)
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
		writeGraphSlots(&b, slots[:2*(lastCol+1)], cs)
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
	writeGraphSlots(&b, slots[:2*(lastActive+1)], cs)

	// Merge commits get (n-1)*2 extra space chars after the `*` slot to
	// align with the stagger row above. Matches git's --graph output.
	if extra := len(row.Commit.Parents) - 1; extra > 0 {
		b.WriteString(csCall(cs, KindGraph, strings.Repeat(" ", extra*2)))
	}

	// Split label into hash, refs, and subject parts for coloring.
	// Expected format: "<hash> <refs> <subject>" or "<hash> <subject>"
	// where refs are wrapped in "(...)" if present.
	label := row.Commit.Label
	hashEnd := strings.IndexByte(label, ' ')
	if hashEnd < 0 {
		b.WriteString(csCall(cs, KindHash, label))
		return b.String()
	}
	hash := label[:hashEnd]
	rest := label[hashEnd+1:]

	b.WriteString(csCall(cs, KindHash, hash))

	if len(rest) > 0 && rest[0] == '(' {
		// There's a ref decoration.
		refEnd := strings.IndexByte(rest, ')')
		if refEnd >= 0 {
			refs := rest[:refEnd+1]
			subject := strings.TrimLeft(rest[refEnd+1:], " ")
			b.WriteByte(' ')
			b.WriteString(csCall(cs, KindRef, refs))
			if subject != "" {
				b.WriteByte(' ')
				b.WriteString(csCall(cs, KindSubject, subject))
			}
		} else {
			b.WriteByte(' ')
			b.WriteString(csCall(cs, KindSubject, rest))
		}
	} else if rest != "" {
		b.WriteByte(' ')
		b.WriteString(csCall(cs, KindSubject, rest))
	}

	return b.String()
}
