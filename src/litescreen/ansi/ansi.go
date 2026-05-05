// Package ansi parses SGR (Select Graphic Rendition) escape sequences out
// of a string and produces a stream of styled runes suitable for feeding
// into a cell-based renderer like litescreen.
//
// Scope: SGR (CSI ... m) sequences only. Other escape sequences (cursor
// moves, OSC, mode changes) are recognised by their CSI/OSC framing and
// discarded -- they don't produce runes and don't affect the carried
// style. Bare bytes pass through as runes (UTF-8 decoded).
//
// Reset semantics: SGR 0 (or empty params, which CSI treats as 0) resets
// the carried style to the base style passed to Parse / WriteToScreen.
package ansi

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

// StyledRune is a rune with its computed style after SGR application.
type StyledRune struct {
	R     rune
	Style tcell.Style
}

// Parse converts s into a slice of styled runes. base is the starting style
// applied before any SGR sequence; SGR 0 / empty params reset back to it.
// Non-SGR escapes are skipped (consumed, discarded).
func Parse(s string, base tcell.Style) []StyledRune {
	out := make([]StyledRune, 0, len(s))
	emit := func(r rune, st tcell.Style) {
		out = append(out, StyledRune{R: r, Style: st})
	}
	walk(s, base, emit)
	return out
}

// WriteToScreen feeds runes from s into set, starting at (x0, y) and
// advancing rightwards. Newlines (\n) move to (x0, y+1). Carriage returns
// (\r) reset column to x0 without changing row. Returns the final cursor
// position after the last rune.
func WriteToScreen(set func(x, y int, r rune, style tcell.Style), x0, y int, s string, base tcell.Style) (int, int) {
	x := x0
	walk(s, base, func(r rune, st tcell.Style) {
		switch r {
		case '\n':
			x = x0
			y++
		case '\r':
			x = x0
		default:
			set(x, y, r, st)
			x++
		}
	})
	return x, y
}

// walk drives the parser, calling emit for every rune (including newlines)
// with its associated style. Pure -- no allocations beyond what the
// callback does.
func walk(s string, base tcell.Style, emit func(rune, tcell.Style)) {
	cur := base
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b && i+1 < len(s) {
			consumed, newStyle, ok := handleEscape(s[i:], cur, base)
			if ok {
				cur = newStyle
				i += consumed
				continue
			}
			// Unrecognised escape; consume just the ESC and emit it raw
			// would corrupt downstream rendering. Drop the ESC byte.
			i++
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		if sz == 0 {
			// Defensive: malformed UTF-8, skip the byte.
			i++
			continue
		}
		emit(r, cur)
		i += sz
	}
}

// handleEscape inspects an escape sequence at the start of s (which begins
// with ESC). Returns bytes consumed, the new style, and whether the
// sequence was recognised. Unknown sequences still return ok=true with
// the carried style, so the caller can advance past them.
func handleEscape(s string, cur, base tcell.Style) (int, tcell.Style, bool) {
	if len(s) < 2 {
		return 0, cur, false
	}
	switch s[1] {
	case '[':
		return handleCSI(s, cur, base)
	case ']':
		return handleOSC(s), cur, true
	default:
		// Two-char escape (ESC X). Consume both, ignore.
		return 2, cur, true
	}
}

// handleCSI parses a CSI sequence starting at s[0] == ESC, s[1] == '['.
// Returns bytes consumed (including the final byte) and the post-sequence
// style. Only SGR ('m') sequences mutate style; others are consumed
// untouched.
func handleCSI(s string, cur, base tcell.Style) (int, tcell.Style, bool) {
	// Find final byte in range 0x40..0x7E. Params are everything between
	// '[' and the final byte (intermediate bytes 0x20..0x2F allowed but
	// rare for SGR).
	i := 2
	for i < len(s) {
		c := s[i]
		if c >= 0x40 && c <= 0x7e {
			final := c
			params := s[2:i]
			if final == 'm' {
				return i + 1, applySGR(params, cur, base), true
			}
			return i + 1, cur, true
		}
		i++
	}
	// No final byte found; treat as malformed and consume everything.
	return len(s), cur, true
}

// handleOSC consumes an OSC sequence (ESC ] ... ST/BEL). Returns bytes
// consumed. OSC has no effect on SGR style.
func handleOSC(s string) int {
	i := 2
	for i < len(s) {
		// BEL terminator
		if s[i] == 0x07 {
			return i + 1
		}
		// ST terminator: ESC \
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return len(s)
}

// applySGR processes a semicolon-separated parameter string from a CSI ... m
// sequence and mutates the carried style accordingly. Empty params
// (e.g. "\x1b[m") are treated as SGR 0 / reset.
func applySGR(params string, cur, base tcell.Style) tcell.Style {
	if params == "" {
		return base
	}
	parts := strings.Split(params, ";")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0 // empty / non-numeric -> 0 per ANSI convention
		}
		nums = append(nums, n)
	}

	st := cur
	for i := 0; i < len(nums); i++ {
		n := nums[i]
		switch {
		case n == 0:
			st = base
		case n == 1:
			st = st.Bold(true)
		case n == 2:
			st = st.Dim(true)
		case n == 3:
			st = st.Italic(true)
		case n == 4:
			st = st.Underline(true)
		case n == 5, n == 6:
			st = st.Blink(true)
		case n == 7:
			st = st.Reverse(true)
		case n == 9:
			st = st.StrikeThrough(true)
		case n == 22:
			st = st.Bold(false).Dim(false)
		case n == 23:
			st = st.Italic(false)
		case n == 24:
			st = st.Underline(false)
		case n == 25:
			st = st.Blink(false)
		case n == 27:
			st = st.Reverse(false)
		case n == 29:
			st = st.StrikeThrough(false)
		case n >= 30 && n <= 37:
			st = st.Foreground(tcell.PaletteColor(n - 30))
		case n == 38:
			color, consumed, ok := readExtendedColor(nums[i+1:])
			if ok {
				st = st.Foreground(color)
				i += consumed
			}
		case n == 39:
			st = st.Foreground(tcell.ColorDefault)
		case n >= 40 && n <= 47:
			st = st.Background(tcell.PaletteColor(n - 40))
		case n == 48:
			color, consumed, ok := readExtendedColor(nums[i+1:])
			if ok {
				st = st.Background(color)
				i += consumed
			}
		case n == 49:
			st = st.Background(tcell.ColorDefault)
		case n >= 90 && n <= 97:
			st = st.Foreground(tcell.PaletteColor(n - 90 + 8))
		case n >= 100 && n <= 107:
			st = st.Background(tcell.PaletteColor(n - 100 + 8))
		}
	}
	return st
}

// StyleToSGR encodes a tcell.Style as the corresponding SGR escape sequence.
// Returns "" for the default style (no attrs, default fg+bg) so callers can
// skip writes when nothing needs to change. Inverse of the Parse direction.
func StyleToSGR(st tcell.Style) string {
	fg, bg, attr := st.Decompose()
	var params []string
	if attr&tcell.AttrBold != 0 {
		params = append(params, "1")
	}
	if attr&tcell.AttrDim != 0 {
		params = append(params, "2")
	}
	if attr&tcell.AttrItalic != 0 {
		params = append(params, "3")
	}
	if attr&tcell.AttrUnderline != 0 {
		params = append(params, "4")
	}
	if attr&tcell.AttrBlink != 0 {
		params = append(params, "5")
	}
	if attr&tcell.AttrReverse != 0 {
		params = append(params, "7")
	}
	if attr&tcell.AttrStrikeThrough != 0 {
		params = append(params, "9")
	}
	switch {
	case fg == tcell.ColorDefault:
	case fg > tcell.Color255:
		r, g, b := fg.RGB()
		params = append(params, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("38;5;%d", fg-tcell.ColorValid))
	}
	switch {
	case bg == tcell.ColorDefault:
	case bg > tcell.Color255:
		r, g, b := bg.RGB()
		params = append(params, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("48;5;%d", bg-tcell.ColorValid))
	}
	if len(params) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(params, ";") + "m"
}

// Strip removes ANSI escape sequences from s, returning only the rune
// payload. Convenience for callers that want plain text without styling.
func Strip(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	walk(s, tcell.StyleDefault, func(r rune, _ tcell.Style) {
		b.WriteRune(r)
	})
	return b.String()
}

// readExtendedColor parses the tail of a 38/48 color specifier. Accepts:
//
//	5;N         -> 256-color palette index
//	2;R;G;B     -> truecolor RGB
//
// Returns the parsed Color, number of params consumed (after the 38/48),
// and whether parsing succeeded. Caller advances its loop index by the
// returned count.
func readExtendedColor(rest []int) (tcell.Color, int, bool) {
	if len(rest) == 0 {
		return tcell.ColorDefault, 0, false
	}
	switch rest[0] {
	case 5:
		if len(rest) < 2 {
			return tcell.ColorDefault, 0, false
		}
		n := rest[1]
		if n < 0 || n > 255 {
			return tcell.ColorDefault, 2, false
		}
		return tcell.PaletteColor(n), 2, true
	case 2:
		if len(rest) < 4 {
			return tcell.ColorDefault, 0, false
		}
		return tcell.NewRGBColor(int32(rest[1]), int32(rest[2]), int32(rest[3])), 4, true
	}
	return tcell.ColorDefault, 0, false
}
