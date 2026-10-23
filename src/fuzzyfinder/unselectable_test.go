package fuzzyfinder_test

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

// Unselectable items can't be confirmed (Enter) or toggled (Tab); a
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
		res, err := f.Find(context.Background(), feedOf(it, unsel), ff.Opt{})
		require.NoError(t, err)
		assert.Equal(t, ff.ID(1), res.IDs[0])
	})

	t.Run("enter ignored while selectable matches remain", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// No query, so the cursor rests on the unselectable header-x while
		// apple and banana still match. Enter must noop, then Esc aborts.
		term.SetEvents(enter, esc)

		it := items
		_, err := f.Find(context.Background(), feedOf(it, unsel), ff.Opt{})
		assert.ErrorIs(t, err, ff.ErrAbort)
	})

	t.Run("enter ends the picker when no match is selectable", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// Query isolates the unselectable item: the picker cannot produce a
		// selection, so Enter ends it empty rather than becoming a dead key.
		term.SetEvents(append(runes("header"), enter)...)

		it := items
		res, err := f.Find(context.Background(), feedOf(it, unsel), ff.Opt{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(res.IDs))
		assert.Equal(t, "header", res.Query)
	})

	t.Run("enter ends the picker when nothing matches", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		term.SetEvents(append(runes("zzz"), enter)...)

		it := items
		res, err := f.Find(context.Background(), feedOf(it, unsel), ff.Opt{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(res.IDs))
		assert.Equal(t, "zzz", res.Query)
	})

	t.Run("every item unselectable makes enter a plain query read", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// The shape ui.Prompt relies on: nothing is ever selectable, so Enter
		// always hands back the query. Items still match, as collision feedback.
		term.SetEvents(append(runes("appl"), enter)...)

		it := items
		res, err := f.Find(context.Background(), feedOf(it, func(string) bool { return true }), ff.Opt{})
		require.NoError(t, err)
		assert.Equal(t, 0, len(res.IDs))
		assert.Equal(t, "appl", res.Query)
	})

	t.Run("tab on unselectable advances cursor without selecting", func(t *testing.T) {
		t.Parallel()
		f, term := ff.NewWithMockedTerminal()
		// Cursor starts on header-x (idx 0). First Tab can't select it but
		// advances to apple (idx 1); second Tab selects apple; Enter confirms.
		term.SetEvents(tab, tab, enter)

		it := items
		res, err := f.Find(context.Background(), feedOf(it, unsel), ff.Opt{Multi: true})
		require.NoError(t, err)
		assert.EqualArrays(t, []ff.ID{1}, res.IDs)
	})
}

// feedOf builds a closed feed over items, flagging those unsel picks.
func feedOf(items []string, unsel func(string) bool) *ff.Feed {
	feed := ff.NewFeed()
	for _, it := range items {
		var flags ff.Flags
		if unsel(it) {
			flags = ff.Unselectable
		}
		feed.AddWith(it, flags)
	}
	feed.Close()
	return feed
}
