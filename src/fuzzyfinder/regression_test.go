package fuzzyfinder_test

import (
	"context"
	"testing"

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
	res, err := f.Find(context.Background(), &it, nil, ff.Opt{Multi: true})
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
	res, err := f.Find(context.Background(), &it, nil, ff.Opt{Query: query})
	require.NoError(t, err)
	assert.Equal(t, acute+acute, res.Query)
}
