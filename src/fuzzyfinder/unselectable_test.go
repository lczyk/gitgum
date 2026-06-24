package fuzzyfinder_test

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// Opt.Unselectable items can't be confirmed (Enter) or toggled (Tab); a
// selectable item still confirms normally.
func TestUnselectable(t *testing.T) {
	t.Parallel()

	items := []string{"header-x", "apple", "banana"}
	unsel := func(s string) bool { return s == "header-x" }

	enter := key(input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone})
	tab := key(input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone})
	esc := key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone})

	t.Run("enter confirms selectable", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		term.SetEvents(append(runes("apple"), enter)...)

		it := items
		idxs, err := f.Find(context.Background(), &it, nil, ff.Opt{Unselectable: unsel})
		require.NoError(t, err)
		assert.Equal(t, 1, idxs[0])
	})

	t.Run("enter ignored on unselectable", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// Query isolates the unselectable item. Enter must noop, then Esc aborts.
		term.SetEvents(append(runes("header"), enter, esc)...)

		it := items
		_, err := f.Find(context.Background(), &it, nil, ff.Opt{Unselectable: unsel})
		assert.ErrorIs(t, err, ff.ErrAbort)
	})

	t.Run("tab on unselectable advances cursor without selecting", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// Cursor starts on header-x (idx 0). First Tab can't select it but
		// advances to apple (idx 1); second Tab selects apple; Enter confirms.
		term.SetEvents(tab, tab, enter)

		it := items
		idxs, err := f.Find(context.Background(), &it, nil, ff.Opt{Multi: true, Unselectable: unsel})
		require.NoError(t, err)
		assert.EqualArrays(t, []int{1}, idxs)
	})
}
