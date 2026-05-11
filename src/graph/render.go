package graph

import "unsafe"

// Render produces output lines in `git log --graph --oneline` style.
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
		if row.Commit != nil {
			estBytes += len(row.Commit.Label) + row.Extras*2 + 1
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
	// primary slides up too -- this is git's compressed `|\|` cross-routing
	// pattern. Without packing, multi-col layouts get extra whitespace.
	slots := (*slotsBuf)[:0]
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
	*slotsBuf = slots

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
			return buf
		}
	} else if lastActive < 0 {
		lastActive = 0
	}

	slotEnd := 2 * (lastActive + 1)
	// Tail decorations sit immediately after the last non-space slot, used
	// by stagger rows needing a transient extra glyph (e.g. the trailing
	// `|` in git's `|\|` weave) without growing numCols. Trim col-pad
	// spaces so the tail abuts the last meaningful slot.
	if len(row.Tail) > 0 {
		for slotEnd > 0 && slots[slotEnd-1] == GlyphSpace {
			slotEnd--
		}
	}
	buf = writeSlotsTo(buf, slots[:slotEnd], st)
	if len(row.Tail) > 0 {
		buf = writeSlotsTo(buf, row.Tail, st)
	}

	if row.Commit == nil {
		return buf
	}

	// Merge commits get extra alignment slots after the `*` to line up with
	// fan-out cols above (each fan-out parent contributes 2 chars). Layout
	// pre-computes this from actual parent col positions.
	for range row.Extras * 2 {
		buf = append(buf, ' ')
	}

	// Label is opaque -- callers embed ANSI codes pre-Layout if they want
	// per-segment coloring.
	buf = append(buf, row.Commit.Label...)
	return buf
}

// writeSlotsTo emits glyph runs of identical Glyph as a single styled
// write. Lines (`|`/`/`/`\`) are wrapped with Style.LinePrefix/LineSuffix,
// stars with Style.StarPrefix/StarSuffix; spaces and unstyled cases go
// straight to the buffer.
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
		case GlyphStar:
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
