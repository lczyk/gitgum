package fuzzyfinder_test

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

const multiOrderReps = 50

// TestFindMulti_SelectionOrder pins Result.Indices to the order the user marked
// items in, not to index order, and pins it to be the same on every run.
//
// Both the shape and the repetition are load-bearing. Four selections marked
// bottom-up make the expected order the exact reverse of index order, so
// neither an index-sorted nor a partly scrambled result can pass by accident;
// two would prove nothing, since a two-element sort makes one comparison and
// both outcomes are right. And because the selection lives in a map, the order
// the entries are collected in is randomised, so the defect this pins produces
// a wrong answer only for some starting orders -- one run passes against the
// broken code about half the time. Repeating lets the test decide, not the map.
func TestFindMulti_SelectionOrder(t *testing.T) {
	t.Parallel()

	up := input{tcell.KeyUp, rune(tcell.KeyUp), tcell.ModNone}
	down := input{tcell.KeyDown, rune(tcell.KeyDown), tcell.ModNone}
	tab := input{tcell.KeyTab, rune(tcell.KeyTab), tcell.ModNone}
	enter := input{tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone}

	names := trackNames()
	require.That(t, len(names) >= 4, "fixture needs at least 4 items")

	for rep := range multiOrderReps {
		// Tab marks the cursored item and then advances one row, so each hop
		// is spelled out relative to where the previous Tab left the cursor.
		events := keys(
			up, up, up, tab, // cursor 0 -> 3, mark 3, cursor -> 4
			down, down, tab, // cursor 4 -> 2, mark 2, cursor -> 3
			down, down, tab, // cursor 3 -> 1, mark 1, cursor -> 2
			down, down, tab, // cursor 2 -> 0, mark 0, cursor -> 1
			enter,
		)

		f, term := ff.NewWithMockedTerminal()
		term.SetEvents(events...)

		res, err := f.Find(context.Background(), ff.NewSliceSourceFrom(names), ff.Opt{Multi: true})
		require.NoError(t, err, "rep ", rep)
		assert.EqualArrays(t, []int{3, 2, 1, 0}, res.Indices)
		assert.EqualArrays(t, []string{names[3], names[2], names[1], names[0]}, res.Items)
	}
}

// TestReadKey_CtrlWKeepsTextAfterCursor pins ctrl+w to deleting only the word
// before the cursor. It used to truncate the whole query at the word boundary,
// throwing away everything to the right of the cursor along with it.
func TestReadKey_CtrlWKeepsTextAfterCursor(t *testing.T) {
	t.Parallel()

	left := input{tcell.KeyLeft, rune(tcell.KeyLeft), tcell.ModNone}
	events := runes("foo bar")
	// Park the cursor just after "foo", leaving " bar" to its right.
	events = append(events, keys(left, left, left, left)...)
	events = append(events, key(input{tcell.KeyCtrlW, rune(tcell.KeyCtrlW), tcell.ModNone}))
	events = append(events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(events...)

	res, err := f.Find(context.Background(), ff.NewSliceSourceFrom(trackNames()), ff.Opt{})
	require.Error(t, err, ff.ErrAbort)
	assert.Equal(t, " bar", res.Query)
}

// TestReadKey_CtrlWAtStartOfLineIsNoop pins the no-word-before-cursor case:
// there is nothing to kill, so the query is left exactly as typed rather than
// being cleared.
func TestReadKey_CtrlWAtStartOfLineIsNoop(t *testing.T) {
	t.Parallel()

	home := input{tcell.KeyHome, rune(tcell.KeyHome), tcell.ModNone}
	events := runes("foo")
	events = append(events, key(home))
	events = append(events, key(input{tcell.KeyCtrlW, rune(tcell.KeyCtrlW), tcell.ModNone}))
	events = append(events, key(input{tcell.KeyEsc, rune(tcell.KeyEsc), tcell.ModNone}))

	f, term := ff.NewWithMockedTerminal()
	term.SetEvents(events...)

	res, err := f.Find(context.Background(), ff.NewSliceSourceFrom(trackNames()), ff.Opt{})
	require.Error(t, err, ff.ErrAbort)
	assert.Equal(t, "foo", res.Query)
}
