package fuzzyfinder

import (
	"testing"

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
