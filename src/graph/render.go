package graph

import "unsafe"

// Render produces output lines: graph glyphs, then a space, then the row's
// node Label (in the style of `git log --graph --oneline` output).
// Style controls graph-glyph ANSI wrapping; pass the zero Style for plain
// ASCII. Labels are appended verbatim -- callers wanting per-segment
// coloring should embed ANSI escapes in Node.Label before calling Layout.
//
// Internally all lines are written into one shared []byte and the
// returned strings alias substrings of it. That keeps the alloc count
// O(1) in number of rows (a single backing buffer + the []string header
// plus a slots scratch slice), rather than O(rows).
func Render(lr LayoutResult, st Style) []string {
	if len(lr.Rows) == 0 {
		return nil
	}
	// Reused across rows; renderRowInto truncates to zero before refilling.
	slots := make([]Glyph, 0, 2*lr.Columns+4)
	// Rough estimate: 2 chars/col + per-row label budget. Over-allocate
	// modestly so the buffer rarely grows -- a few growslice events are
	// much cheaper than one alloc per row.
	estBytes := 0
	for _, row := range lr.Rows {
		estBytes += 2 * lr.Columns
		if row.Node != nil {
			estBytes += len(row.Node.Label) + 1
		}
		if styleOverhead := len(st.LinePrefix) + len(st.LineSuffix) + len(st.StarPrefix) + len(st.StarSuffix); styleOverhead > 0 {
			estBytes += styleOverhead * 4
		}
	}
	buf := make([]byte, 0, estBytes)
	offsets := make([]int, len(lr.Rows)+1)
	for i, row := range lr.Rows {
		offsets[i] = len(buf)
		buf = renderRowInto(buf, &slots, row, lr.Columns, st)
	}
	offsets[len(lr.Rows)] = len(buf)

	lines := make([]string, len(lr.Rows))
	for i := range lines {
		start, end := offsets[i], offsets[i+1]
		if start == end {
			continue // leave as ""
		}
		// unsafe.String aliases the backing array. Strings are
		// immutable so the caller cannot mutate buf via the lines;
		// buf itself is never mutated after this loop either.
		lines[i] = unsafe.String(&buf[start], end-start)
	}
	return lines
}

func renderRowInto(buf []byte, slotsBuf *[]Glyph, row Row, numCols int, st Style) []byte {
	// Build slot grid by packing left-to-right. Diagonals (`/`, `\`) slide
	// into the previous col's trailing-space slot, and the next col's
	// primary slides up too -- the compressed `|\|` cross-routing pattern
	// (as in git's graph.c). Without packing, multi-col layouts get extra
	// whitespace.
	slots := (*slotsBuf)[:0]
	for c := 0; c < numCols; c++ {
		g := row.Glyphs[c]
		// The row-walker emits a clean fixed grid (2 slots per column). The
		// trailing half-slot is normally a space, but the walker can place a
		// crossing diagonal there (Gap) so it weaves between two intact pipes.
		trailing := GlyphSpace
		if c < len(row.Gap) {
			trailing = row.Gap[c]
		}
		slots = append(slots, g, trailing)
	}
	for len(slots) < 2*numCols {
		slots = append(slots, GlyphSpace)
	}
	*slotsBuf = slots

	// Determine right edge: stagger rows render up to the rightmost col with
	// non-space content; node rows do the same (with at least col 0).
	lastActive := -1
	for c := 0; c < numCols; c++ {
		if row.Glyphs[c] != GlyphSpace {
			lastActive = c
		}
	}
	// Gaps hold crossing diagonals a half-slot right of their column. One that
	// sits past the last active pipe -- a birth diagonal over a not-yet-settled
	// lane, or an edge routed across an empty column -- still has to render.
	// Counting only primary glyphs would truncate it off the right edge,
	// dropping the diagonal and leaving a bare pipe row (the diagonal then
	// appears to skip a column each step, only surfacing on its even-slot rows).
	lastGap := -1
	for c := 0; c < numCols && c < len(row.Gap); c++ {
		if row.Gap[c] != GlyphSpace {
			lastGap = c
		}
	}
	if row.Node == nil {
		if lastActive < 0 && lastGap < 0 {
			return buf
		}
	} else if lastActive < 0 {
		lastActive = 0
	}

	slotEnd := 2 * (lastActive + 1) // through the last active column's trailing gap
	if g := 2*lastGap + 2; g > slotEnd {
		slotEnd = g // extend to cover a diagonal in a trailing gap slot
	}
	buf = writeSlotsTo(buf, slots[:slotEnd], st)

	if row.Node == nil {
		return buf
	}

	// Label is opaque -- callers embed ANSI codes pre-Layout if they want
	// per-segment coloring.
	buf = append(buf, row.Node.Label...)
	return buf
}

// writeSlotsTo emits glyph runs of identical Glyph as a single styled
// write. Lines (`|`/`/`/`\`) are wrapped with Style.LinePrefix/LineSuffix,
// stars with Style.StarPrefix/StarSuffix; spaces and unstyled cases go
// straight to the buffer. Split markers (`v`/`^`) stand in for a node that
// isn't there, so they take the star's styling.
func writeSlotsTo(buf []byte, slots []Glyph, st Style) []byte {
	if len(slots) == 0 {
		return buf
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
				buf = append(buf, ' ')
			}
		case GlyphStar, GlyphV, GlyphCaret:
			if st.StarPrefix == "" && st.StarSuffix == "" {
				for range n {
					buf = append(buf, ch...)
				}
			} else {
				buf = append(buf, st.StarPrefix...)
				for range n {
					buf = append(buf, ch...)
				}
				buf = append(buf, st.StarSuffix...)
			}
		default: // pipe, slash, backslash
			if st.LinePrefix == "" && st.LineSuffix == "" {
				for range n {
					buf = append(buf, ch...)
				}
			} else {
				buf = append(buf, st.LinePrefix...)
				for range n {
					buf = append(buf, ch...)
				}
				buf = append(buf, st.LineSuffix...)
			}
		}
		runStart = i
	}
	return buf
}
