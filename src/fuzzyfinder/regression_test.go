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
