package fuzzyfinder

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

func TestParseAnsiItems_StripsAndStyles(t *testing.T) {
	items := []string{
		"\x1b[31mred\x1b[0m",
		"plain",
		"\x1b[1mbold\x1b[m",
	}
	stripped, styled := parseAnsiItems(items)

	assert.EqualArrays(t, stripped, []string{"red", "plain", "bold"})
	assert.Equal(t, len(styled), 3)

	// First item: 3 runes, all red, so one span carries the whole item.
	assert.Equal(t, len(styled[0]), 1)
	fg, _, _ := ansi.StyleAt(styled[0], 2, tcell.StyleDefault).Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1))

	// Second item: plain runes, default style.
	assert.Equal(t, ansi.StyleAt(styled[1], 4, tcell.StyleDefault), tcell.StyleDefault)

	// Third item: bold attr.
	_, _, attr := ansi.StyleAt(styled[2], 3, tcell.StyleDefault).Decompose()
	assert.That(t, attr&tcell.AttrBold != 0, "expected bold")
}

func TestParseAnsiItems_Empty(t *testing.T) {
	stripped, styled := parseAnsiItems(nil)
	assert.Equal(t, len(stripped), 0)
	assert.Equal(t, len(styled), 0)
}

// initFinder w/ Opt.Ansi populates state.itemsStyled and stores stripped
// items for matching.
func TestInitFinder_AnsiPopulatesStyledItems(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	items := []string{"\x1b[31mhello\x1b[0m", "world"}
	err := f.initFinder(items, Opt{Ansi: true})
	require.NoError(t, err)

	assert.EqualArrays(t, f.state.items, []string{"hello", "world"})
	assert.Equal(t, len(f.state.itemsStyled), 2)
	// "hello" is red through its last rune.
	fg, _, _ := ansi.StyleAt(f.state.itemsStyled[0], 4, tcell.StyleDefault).Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1))
}

// The drawing path reads colours out of the style spans, so an item that isn't
// under the cursor (which overrides styling) renders in the colour its ANSI
// asked for. Items start at column 2.
func TestDraw_AnsiItemKeepsItsColour(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	items := []string{"plain", "\x1b[31mred\x1b[0m"}
	require.NoError(t, f.initFinder(items, Opt{Ansi: true}))
	f._draw()

	_, h := m.Size()
	found := false
	for y := range h {
		text, style, _ := m.Get(2, y)
		if text != "r" {
			continue
		}
		found = true
		fg, _, _ := style.Decompose()
		assert.Equal(t, fg, tcell.PaletteColor(1))
	}
	assert.That(t, found, "expected a drawn row starting with the red item")
}

// initFinder without Opt.Ansi leaves itemsStyled nil and aliases input.
func TestInitFinder_NoAnsi(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	items := []string{"plain"}
	err := f.initFinder(items, Opt{})
	require.NoError(t, err)

	assert.Nil(t, f.state.itemsStyled, "itemsStyled should be nil without Opt.Ansi")
	assert.EqualArrays(t, f.state.items, items)
}

func TestNegRuneMask(t *testing.T) {
	// negate off -> nil regardless of input.
	assert.Nil(t, negRuneMask([]rune("!foo"), false))
	// no negated term -> nil.
	assert.Nil(t, negRuneMask([]rune("foo bar"), true))

	// "foo !bar": only the "!bar" runes (indices 4-7) are masked.
	got := negRuneMask([]rune("foo !bar"), true)
	want := []bool{false, false, false, false, true, true, true, true}
	assert.EqualArrays(t, want, got)
}
