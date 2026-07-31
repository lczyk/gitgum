package ansi_test

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

func TestParseStyles_RunsCollapse(t *testing.T) {
	got := ansi.ParseStyles("\x1b[31mRR\x1b[0mPP", tcell.StyleDefault)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].Start, 0)
	assert.Equal(t, got[1].Start, 2)
	assert.Equal(t, got[1].Style, tcell.StyleDefault)

	fg, _, _ := got[0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1))
}

func TestParseStyles_PlainTextIsOneSpan(t *testing.T) {
	got := ansi.ParseStyles("abc", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].Start, 0)
	assert.Equal(t, got[0].Style, tcell.StyleDefault)
}

// A style change with no runes after it produces no span: spans are keyed by
// the runes they cover, and there are none.
func TestParseStyles_NoRunes(t *testing.T) {
	assert.Nil(t, ansi.ParseStyles("", tcell.StyleDefault))
	assert.Nil(t, ansi.ParseStyles("\x1b[31m", tcell.StyleDefault))
}

// Repeating a style that is already current doesn't open a span.
func TestParseStyles_RedundantSGRDoesNotSplit(t *testing.T) {
	got := ansi.ParseStyles("\x1b[31mR\x1b[31mR", tcell.StyleDefault)
	assert.Equal(t, len(got), 1)
}

func TestStyleAt(t *testing.T) {
	base := tcell.StyleDefault.Bold(true)
	spans := ansi.ParseStyles("\x1b[31mRR\x1b[32mGG", tcell.StyleDefault)

	red, _, _ := ansi.StyleAt(spans, 1, base).Decompose()
	assert.Equal(t, red, tcell.PaletteColor(1))
	green, _, _ := ansi.StyleAt(spans, 2, base).Decompose()
	assert.Equal(t, green, tcell.PaletteColor(2))

	// Past the last span the last style still applies -- it runs to the end.
	green, _, _ = ansi.StyleAt(spans, 99, base).Decompose()
	assert.Equal(t, green, tcell.PaletteColor(2))

	// Nothing parsed means nothing to say; the caller's base stands.
	assert.Equal(t, ansi.StyleAt(nil, 0, base), base)
}

// ParseStyles never leaves a gap before the first span -- its first rune opens
// one at index 0 -- but StyleAt is public and promises base ahead of the first
// span, so hand-built spans are the only way to hold it to that.
func TestStyleAt_BeforeFirstSpan(t *testing.T) {
	base := tcell.StyleDefault.Bold(true)
	spans := []ansi.StyleSpan{
		{Start: 2, Style: tcell.StyleDefault.Foreground(tcell.PaletteColor(1))},
		{Start: 5, Style: tcell.StyleDefault.Foreground(tcell.PaletteColor(2))},
	}

	assert.Equal(t, ansi.StyleAt(spans, 0, base), base)
	assert.Equal(t, ansi.StyleAt(spans, 1, base), base)

	red, _, _ := ansi.StyleAt(spans, 2, base).Decompose()
	assert.Equal(t, red, tcell.PaletteColor(1))
	green, _, _ := ansi.StyleAt(spans, 5, base).Decompose()
	assert.Equal(t, green, tcell.PaletteColor(2))
}

// ParseStyles must answer for every rune index exactly what Parse records
// there. The picker holds spans instead of styled runes on the strength of
// that equivalence, so it is the property worth pinning.
func TestParseStyles_AgreesWithParse(t *testing.T) {
	inputs := []string{
		"abc",
		"\x1b[31mR\x1b[0mP",
		"\x1b[1;33mwarn\x1b[m plain \x1b[38;5;200mpink",
		"\x1b[31mé✓\x1b[0mx", // multibyte: span indices count runes
		"a\x1b[5Ab",
		"a\x1b]0;title\x07b",
		"a\nb",
		"\x1b[31mR\x1b[31mR",
	}
	base := tcell.StyleDefault.Dim(true)
	for _, in := range inputs {
		runes := ansi.Parse(in, base)
		spans := ansi.ParseStyles(in, base)
		for i, sr := range runes {
			assert.Equal(t, ansi.StyleAt(spans, i, base), sr.Style,
				"index %d of %q", i, in)
		}
	}
}
