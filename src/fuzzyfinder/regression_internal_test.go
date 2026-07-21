package fuzzyfinder

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// _draw must tolerate state.matched holding indices past len(items). A
// filter()/updateItems() interleave on a shrinking live source can leave
// matched pointing at a former, larger item slice for one frame; the raw
// items[m] read then indexed out of range and crashed the picker mid-render.
func TestRegressionDrawStaleMatchedNoPanic(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	require.NoError(t, f.initFinder([]string{"a", "b"}, Opt{}))

	// matched valid for a 10-item slice; items has since shrunk to 2.
	f.state.matched = []int{0, 5, 1, 9}
	f.state.y = 0
	f.state.cursorY = 0

	// Must not panic.
	f._draw()
}

// A resync that lands between Enter-confirmation and index-to-string
// translation must not change which items the Result reports. Regression:
// confirmSelection captured indices under lock but result() re-read
// f.state.items afterwards, so a source swap in the gap returned whatever now
// sat at those indices instead of what was under the cursor at Enter.
func TestRegressionConfirmSnapshotBeatsResync(t *testing.T) {
	f, m := NewWithMockedTerminal()
	defer m.Fini()
	require.NoError(t, f.initFinder([]string{"alpha", "bravo", "charlie"}, Opt{}))
	f.state.y = 1 // cursor on "bravo"

	idxs, done := f.confirmSelection()
	require.That(t, done, "confirmSelection should confirm")

	// Source wholesale-replaced before the caller translates indices.
	f.updateItems([]string{"x", "y", "z"})

	res, err := f.result(idxs, nil)
	require.NoError(t, err)
	assert.EqualArrays(t, []string{"bravo"}, res.Items)
}
