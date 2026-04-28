package fuzzyfinder

import (
	"testing"

	"github.com/lczyk/assert"
)

// newTestFinder builds a finder with state populated for resync-logic unit
// tests. Bypasses initFinder so no terminal screen is required.
func newTestFinder(items []string, multi bool) *finder {
	f := &finder{}
	f.eventCh = make(chan struct{}, 64)
	f.multi = multi
	f.state.items = items
	f.state.matched = makeMatched(len(items))
	if multi {
		f.state.selection = map[int]int{}
		f.state.selectionIdx = 1
	}
	return f
}

func TestUpdateItems_RemovalAboveCursor(t *testing.T) {
	f := newTestFinder([]string{"a", "b", "c"}, false)
	f.state.y = 2 // cursor on "c"

	f.updateItems([]string{"b", "c"})

	assert.Equal(t, f.state.y, 1)
	assert.Equal(t, f.state.items[f.state.matched[f.state.y]], "c")
}

func TestUpdateItems_RemovalBelowCursor(t *testing.T) {
	f := newTestFinder([]string{"a", "b", "c"}, false)
	f.state.y = 0 // cursor on "a"

	f.updateItems([]string{"a", "c"})

	assert.Equal(t, f.state.y, 0)
	assert.Equal(t, f.state.items[f.state.matched[f.state.y]], "a")
}

func TestUpdateItems_RemovalOfCursoredItem(t *testing.T) {
	f := newTestFinder([]string{"a", "b", "c"}, false)
	f.state.y = 1 // cursor on "b"

	f.updateItems([]string{"a", "c"})

	// "b" is gone. Cursor clamps within bounds; key isn't found so falls to
	// clamp logic which keeps y at its current value bounded to len-1.
	assert.That(t, f.state.y >= 0 && f.state.y < len(f.state.matched),
		"y must be in bounds: y=%d matched=%d", f.state.y, len(f.state.matched))
}

func TestUpdateItems_RemovalOfSelected(t *testing.T) {
	f := newTestFinder([]string{"a", "b", "c"}, true)
	f.state.selection = map[int]int{0: 1, 2: 2} // a@1, c@2
	f.state.selectionIdx = 3
	f.state.y = 1 // cursor on "b"

	f.updateItems([]string{"a", "c"})

	assert.Equal(t, len(f.state.selection), 2)
	assert.Equal(t, f.state.selection[0], 1) // "a" still at idx 0, order 1
	assert.Equal(t, f.state.selection[1], 2) // "c" now at idx 1, order 2 preserved
}

func TestUpdateItems_RemovalDropsSelectionForRemovedItems(t *testing.T) {
	f := newTestFinder([]string{"a", "b", "c"}, true)
	f.state.selection = map[int]int{1: 1} // only "b" selected
	f.state.selectionIdx = 2
	f.state.y = 0

	f.updateItems([]string{"a", "c"})

	assert.Equal(t, len(f.state.selection), 0)
}

func TestUpdateItems_DuplicateRemoval(t *testing.T) {
	f := newTestFinder([]string{"x", "x", "y"}, false)
	f.state.y = 1 // cursor on second "x" (key = (x, nth=1))

	f.updateItems([]string{"x", "y"})

	// Surviving "x" has nth=0 in new slice. Old key (x, 1) doesn't resolve;
	// cursor falls to clamp. Documents the v1 semantics.
	assert.That(t, f.state.y >= 0 && f.state.y < len(f.state.matched),
		"y must be in bounds")
}

func TestUpdateItems_DuplicateAcrossSurvivors(t *testing.T) {
	// When the cursored duplicate survives in place, identity matches and y
	// follows it.
	f := newTestFinder([]string{"x", "y", "x"}, false)
	f.state.y = 0 // cursor on first "x" (nth=0)

	f.updateItems([]string{"x", "z", "x"}) // y removed, second x stays at end

	assert.Equal(t, f.state.y, 0)
	assert.Equal(t, f.state.items[f.state.matched[f.state.y]], "x")
}

func TestUpdateItems_AdditionPreservesCursor(t *testing.T) {
	f := newTestFinder([]string{"a", "b"}, false)
	f.state.y = 1 // cursor on "b"

	f.updateItems([]string{"a", "b", "c", "d"})

	assert.Equal(t, f.state.items[f.state.matched[f.state.y]], "b")
}

func TestUpdateItems_AllRemoved(t *testing.T) {
	f := newTestFinder([]string{"a", "b"}, true)
	f.state.selection = map[int]int{0: 1}
	f.state.selectionIdx = 2
	f.state.y = 0

	f.updateItems([]string{})

	assert.Equal(t, len(f.state.matched), 0)
	assert.Equal(t, f.state.y, 0)
	assert.Equal(t, len(f.state.selection), 0)
}

func TestUpdateItems_SignalsRedraw(t *testing.T) {
	f := newTestFinder([]string{"a"}, false)

	f.updateItems([]string{"b"})

	select {
	case <-f.eventCh:
		// drained ok
	default:
		t.Fatal("expected updateItems to enqueue a redraw signal")
	}
}
