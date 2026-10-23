package fuzzyfinder

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// newDeltaFinder builds a finder over items (IDs 0..n-1) for applyDelta unit
// tests.
func newDeltaFinder(t *testing.T, items []string, multi bool) *finder {
	t.Helper()
	f := newHeadlessFinder(multi)
	require.NoError(t, f.initItems(items, Opt{}))
	return f
}

func cursorItem(f *finder) string {
	return f.state.items[f.state.matched[f.state.y]]
}

func TestApplyDelta_RemovalAboveCursor(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, false)
	f.state.y = 2

	f.applyDelta(delta{removed: []ID{0}})

	assert.Equal(t, f.state.y, 1)
	assert.Equal(t, cursorItem(f), "c")
	assert.EqualArrays(t, f.state.ids, []ID{1, 2})
	assert.Equal(t, f.state.pos[2], 1)
}

func TestApplyDelta_RemovalBelowCursor(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, false)
	f.state.y = 0

	f.applyDelta(delta{removed: []ID{1}})

	assert.Equal(t, f.state.y, 0)
	assert.Equal(t, cursorItem(f), "a")
}

func TestApplyDelta_RemovalOfCursoredItemLandsOnSuccessor(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, false)
	f.state.y = 1

	f.applyDelta(delta{removed: []ID{1}})

	assert.Equal(t, cursorItem(f), "c")
}

func TestApplyDelta_RemovalOfLastCursoredItemLandsOnPredecessor(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, false)
	f.state.y = 2

	f.applyDelta(delta{removed: []ID{2}})

	assert.Equal(t, cursorItem(f), "b")
}

// Duplicates are told apart by ID, so removing one copy never moves the
// cursor onto the other.
func TestApplyDelta_DuplicateRemoval(t *testing.T) {
	f := newDeltaFinder(t, []string{"x", "x", "y"}, false)
	f.state.y = 1

	f.applyDelta(delta{removed: []ID{0}})

	assert.Equal(t, f.state.ids[f.state.matched[f.state.y]], ID(1))
}

func TestApplyDelta_RemovalDropsSelection(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, true)
	f.state.selection = map[ID]int{0: 1, 1: 2, 2: 3}

	f.applyDelta(delta{removed: []ID{1}})

	assert.Equal(t, len(f.state.selection), 2)
	assert.Equal(t, f.state.selection[0], 1)
	assert.Equal(t, f.state.selection[2], 3)
}

func TestApplyDelta_AllRemoved(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b"}, true)
	f.state.selection = map[ID]int{0: 1}

	f.applyDelta(delta{removed: []ID{0, 1}})

	assert.Equal(t, len(f.state.matched), 0)
	assert.Equal(t, len(f.state.items), 0)
	assert.Equal(t, f.state.y, 0)
	assert.Equal(t, len(f.state.selection), 0)
}

func TestApplyDelta_AdditionPreservesCursor(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b"}, false)
	f.state.y = 1

	f.applyDelta(delta{added: []entry{{id: 2, text: "c"}, {id: 3, text: "d"}}})

	assert.Equal(t, cursorItem(f), "b")
	assert.EqualArrays(t, f.state.matched, []int{0, 1, 2, 3})
}

func TestApplyDelta_AdditionMatchesOnlyAgainstQuery(t *testing.T) {
	f := newDeltaFinder(t, []string{"apple", "banana"}, false)
	f.state.input = []rune("an")
	f.filter()

	f.applyDelta(delta{added: []entry{{id: 2, text: "mango"}, {id: 3, text: "kiwi"}}})

	assert.EqualArrays(t, f.state.matched, []int{1, 2})
}

func TestApplyDelta_UpdateKeepsCursorAndSelection(t *testing.T) {
	f := newDeltaFinder(t, []string{"a", "b", "c"}, true)
	f.state.y = 1
	f.state.selection = map[ID]int{1: 1}

	f.applyDelta(delta{updated: []entry{{id: 1, text: "B"}}})

	assert.Equal(t, cursorItem(f), "B")
	assert.Equal(t, f.state.selection[1], 1)
	assert.Equal(t, f.state.itemsLower[1], "b")
}

func TestApplyDelta_UpdateOutOfQueryMovesCursorToNextMatch(t *testing.T) {
	f := newDeltaFinder(t, []string{"ax", "bx", "cx"}, false)
	f.state.input = []rune("x")
	f.filter()
	f.state.y = 1

	f.applyDelta(delta{updated: []entry{{id: 1, text: "b"}}})

	assert.EqualArrays(t, f.state.matched, []int{0, 2})
	assert.Equal(t, cursorItem(f), "cx")
}

func TestApplyDelta_UpdateIntoQueryJoinsMatchedInOrder(t *testing.T) {
	f := newDeltaFinder(t, []string{"ax", "b", "cx"}, false)
	f.state.input = []rune("x")
	f.filter()

	f.applyDelta(delta{updated: []entry{{id: 1, text: "bx"}}})

	assert.EqualArrays(t, f.state.matched, []int{0, 1, 2})
}

func TestApplyDelta_AnsiParsesOnlyChangedItems(t *testing.T) {
	f := newHeadlessFinder(false)
	require.NoError(t, f.initItems([]string{"\x1b[31mred\x1b[0m"}, Opt{Ansi: true}))

	f.applyDelta(delta{
		updated: []entry{{id: 0, text: "\x1b[32mgreen\x1b[0m"}},
		added:   []entry{{id: 1, text: "\x1b[1mbold\x1b[m"}},
	})

	assert.EqualArrays(t, f.state.items, []string{"green", "bold"})
	assert.Equal(t, len(f.state.itemsStyled), 2)
}

// A cursor move caused by a delta re-aligns to the page, keeping the
// cursorY == y % pageSize invariant paging relies on.
func TestApplyDelta_MovedCursorStaysPageAligned(t *testing.T) {
	items := make([]string, 20)
	for i := range items {
		items[i] = string(rune('a' + i))
	}
	f := newDeltaFinder(t, items, false)
	f.state.pageSize = 9
	f.state.y = 12
	f.state.cursorY = 3

	f.applyDelta(delta{removed: []ID{0, 1, 2, 3}})

	assert.Equal(t, f.state.y, 8)
	assert.Equal(t, f.state.cursorY, 8)
}

func TestApplyDelta_EmptyIsNoop(t *testing.T) {
	f := newDeltaFinder(t, []string{"a"}, false)
	f.applyDelta(delta{})
	assert.EqualArrays(t, f.state.items, []string{"a"})
}
