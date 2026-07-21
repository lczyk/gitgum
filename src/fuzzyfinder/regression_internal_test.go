package fuzzyfinder

import (
	"context"
	"runtime"
	"testing"
	"time"

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

// When initialisation fails, find() must not leak the resync goroutine.
// Regression: the goroutine was spawned before initFinder and blocked on a
// channel that only closed on success, so an init error left it parked forever.
// (initFinder fails deterministically here because the headless test env has no
// tty for the real screen backend.)
func TestRegressionNoGoroutineLeakOnInitFailure(t *testing.T) {
	src := NewSliceSourceFrom([]string{"a", "b"}) // Versioned: would spawn resync
	before := runtime.NumGoroutine()

	f := &finder{} // nil term -> initFinder builds a real screen and fails
	_, err := f.find(context.Background(), src, Opt{})
	if err == nil {
		t.Fatal("expected init failure with no tty, got nil")
	}

	// No goroutine may outlive the failed init.
	assert.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before
	}, time.Second, 10*time.Millisecond, "resync goroutine leaked after init failure")
}
