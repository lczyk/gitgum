package fuzzyfinder_test

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

// Tab in multi mode while nothing matches must not panic. Regression: the Tab
// handler indexed state.matched[state.y] without guarding the empty-match case,
// so pressing Tab after a no-match query crashed the picker.
func TestRegressionTabOnEmptyMatched(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	tab := key(input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone})
	enter := key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone})
	// "zzz" matches nothing, then Tab (must noop), then Enter ends empty.
	term.SetEvents(append(append(runes("zzz"), tab), enter)...)

	it := []string{"apple", "banana"}
	res, err := f.Find(context.Background(), ff.NewSliceSourceFrom(it), ff.Opt{Multi: true})
	require.NoError(t, err)
	assert.Equal(t, 0, len(res.Indices))
	assert.Equal(t, "zzz", res.Query)
}

// A multibyte Opt.Query must not panic on the first edit. Regression: initFinder
// set state.x to the byte length of the query while x indexes a []rune, so any
// backspace/left/insert on a non-ascii pre-fill indexed past the slice.
func TestRegressionNonAsciiQuery(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	bs := key(input{tcell.KeyBackspace2, rune(tcell.KeyBackspace2), tcell.ModNone})
	enter := key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone})
	// query is three 2-byte runes (U+00E9, "e-acute"); byte len 6 != rune len 3.
	// backspace deletes the last rune, then Enter confirms the query read.
	acute := "\u00e9" // e-acute, U+00E9 (2 bytes)
	query := acute + acute + acute
	term.SetEvents(bs, enter)

	it := []string{query + "x", "plain"}
	res, err := f.Find(context.Background(), ff.NewSliceSourceFrom(it), ff.Opt{Query: query})
	require.NoError(t, err)
	assert.Equal(t, acute+acute, res.Query)
}

// The query-line length cap must count display columns, not runes. Regression:
// the cap compared the rune count against a column budget, so wide (2-column)
// runes overflowed the line -- a full-width run twice as wide as the budget was
// still accepted. At width 12 the budget is 9 columns, so 8 full-width runes
// fit by rune count but must be capped at 4 by column width.
func TestRegressionWideRuneLineCap(t *testing.T) {
	t.Parallel()

	f, term := ff.NewWithMockedTerminal()
	term.SetSize(12, 10) // narrow: column budget is width-3 = 9
	wide := rune(0x3042) // hiragana 'a', 2 columns wide
	esc := key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone})

	events := make([]tcell.Event, 0, 9)
	for range 8 {
		events = append(events, ch(wide))
	}
	events = append(events, esc)
	term.SetEvents(events...)

	it := []string{"placeholder"}
	res, _ := f.Find(context.Background(), ff.NewSliceSourceFrom(it), ff.Opt{})
	assert.Equal(t, 4, utf8.RuneCountInString(res.Query))
}
