package fuzzyfinder

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
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

	// First item: 3 runes, all red.
	assert.Equal(t, len(styled[0]), 3)
	fg, _, _ := styled[0][0].Style.Decompose()
	assert.Equal(t, fg, tcell.PaletteColor(1))

	// Second item: plain runes, default style.
	assert.Equal(t, len(styled[1]), 5)
	assert.Equal(t, styled[1][0].Style, tcell.StyleDefault)

	// Third item: bold attr.
	assert.Equal(t, len(styled[2]), 4)
	_, _, attr := styled[2][0].Style.Decompose()
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
	assert.NoError(t, err)

	assert.EqualArrays(t, f.state.items, []string{"hello", "world"})
	assert.Equal(t, len(f.state.itemsStyled), 2)
	assert.Equal(t, len(f.state.itemsStyled[0]), 5) // "hello"
}

// initFinder without Opt.Ansi leaves itemsStyled nil and aliases input.
func TestInitFinder_NoAnsi(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	items := []string{"plain"}
	err := f.initFinder(items, Opt{})
	assert.NoError(t, err)

	assert.That(t, f.state.itemsStyled == nil, "itemsStyled should be nil without Opt.Ansi")
	assert.EqualArrays(t, f.state.items, items)
}
