package fuzzyfinder

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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

// The resize handler must recompute cursorY with the same page size the rest of
// the picker uses. Regression: after the number-line was merged into the prompt
// row, chrome above the items became 1 row (2 with a header), but the resize
// branch still subtracted 2 (3 with a header) -- an off-by-one page size that
// misaligned the cursor until the next scroll key.
func TestRegressionResizePageSizeOffByOne(t *testing.T) {
	f, m := NewWithMockedTerminal() // 60x10
	items := make([]string, 30)
	for i := range items {
		items[i] = fmt.Sprintf("i%02d", i)
	}
	require.NoError(t, f.initFinder(items, Opt{}))
	f.state.y = 10
	f.state.cursorY = 1

	// Resize to height 12: chrome above items is 1 row, so page size is 11 and
	// the cursored item at y=10 sits on in-page row 10 (10 % 11), not 0.
	m.SetSize(60, 12)
	require.NoError(t, m.PostEvent(tcell.NewEventResize(60, 12)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, f.readKey(ctx))

	assert.Equal(t, 10, f.state.cursorY)
}

// negRuneMask (query-line negation highlight) must match how matching.parse
// decides what is a negated term. Regression: it highlighted a bare "!" that
// parse ignores, and split terms only on space/tab while parse uses
// strings.Fields (all Unicode whitespace).
func TestRegressionNegRuneMaskParityWithParse(t *testing.T) {
	// A lone "!" is not a negated term -> not highlighted.
	assert.Nil(t, negRuneMask([]rune("!"), true))
	assert.Nil(t, negRuneMask([]rune("foo ! bar"), true))

	// A non-breaking space (U+00A0) is a strings.Fields separator, so the "!bc"
	// term after it is recognised and highlighted.
	got := negRuneMask([]rune("a"+"\u00a0"+"!bc"), true)
	want := []bool{false, false, true, true, true} // a, NBSP, !, b, c
	assert.EqualArrays(t, want, got)
}

// repaintScreen counts which flush path the picker took. ChannelEvents parks
// until the test releases it, so the picker stays open for a whole
// measurement window without any key ever arriving.
type repaintScreen struct {
	mu           sync.Mutex
	shows, syncs int
	release      chan struct{}
}

func newRepaintScreen() *repaintScreen {
	return &repaintScreen{release: make(chan struct{})}
}

func (s *repaintScreen) Init() error                                                      { return nil }
func (s *repaintScreen) Fini()                                                            {}
func (s *repaintScreen) Size() (int, int)                                                 { return 40, 10 }
func (s *repaintScreen) Clear()                                                           {}
func (s *repaintScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {}
func (s *repaintScreen) ShowCursor(x, y int)                                              {}
func (s *repaintScreen) Show()                                                            { s.mu.Lock(); s.shows++; s.mu.Unlock() }
func (s *repaintScreen) Sync()                                                            { s.mu.Lock(); s.syncs++; s.mu.Unlock() }

func (s *repaintScreen) ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{}) { <-s.release }

func (s *repaintScreen) counts() (shows, syncs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shows, s.syncs
}

// A full repaint is the expensive path (whole grid re-emitted); it must run
// only when something asked for one. Regression: aggressive mode took it on
// every single frame, so the picker pushed tens of KB at the redraw rate and
// the cursor spent a visible slice of each frame hidden -- a blinking caret
// for as long as the picker was open.
func TestRegressionRepaintOnlyWhenRequested(t *testing.T) {
	t.Parallel()

	scr := newRepaintScreen()
	f := &finder{term: scr, eventCh: make(chan struct{}, 1)}

	for range 3 {
		f.flush()
	}
	shows, syncs := scr.counts()
	assert.Equal(t, syncs, 0)
	assert.Equal(t, shows, 3)

	// A request wakes the draw loop and is spent by exactly one frame; the
	// frames after it drop back to the cheap diff path.
	f.requestRepaint()
	select {
	case <-f.eventCh:
	default:
		t.Fatal("requestRepaint should wake the draw loop")
	}
	f.flush()
	f.flush()
	shows, syncs = scr.counts()
	assert.Equal(t, syncs, 1)
	assert.Equal(t, shows, 4)
}

// End-to-end companion to the above: with RedrawAggressive set and a source
// that never changes, repaints must arrive on repaintInterval, not on the
// resync tick. The bound is deliberately loose -- a loaded machine produces
// fewer ticks, never more -- but still an order of magnitude under the
// ~window/resyncInterval repaints the old code emitted.
func TestRegressionAggressiveRepaintCadence(t *testing.T) {
	t.Parallel()

	const window = 500 * time.Millisecond

	scr := newRepaintScreen()
	defer close(scr.release)

	ctx, cancel := context.WithTimeout(context.Background(), window)
	defer cancel()

	f := &finder{}
	_, err := f.find(ctx, NewSliceSourceFrom([]string{"alpha", "beta"}), Opt{
		Screen:           scr,
		RedrawAggressive: true,
	})
	require.That(t, errors.Is(err, context.DeadlineExceeded), "picker should end on ctx deadline, got %v", err)

	_, syncs := scr.counts()
	maxExpected := int(window/repaintInterval) + 2
	assert.That(t, syncs >= 1, "expected at least one repaint in %v, got %d", window, syncs)
	assert.That(t, syncs <= maxExpected, "expected at most %d repaints in %v, got %d", maxExpected, window, syncs)
}

// A quiet source is not evidence that the producer has exited: its stdout
// (our items) and its stderr (the bytes that tear us) are independent, and
// `find / -name '*.foo'` spends most of its life matching nothing while still
// printing permission errors. So repaints must keep running on cadence even
// though nothing new arrives -- exactly the case this exercises.
func TestRepaintContinuesWhileSourceIsStill(t *testing.T) {
	t.Parallel()

	const window = 700 * time.Millisecond

	scr := newRepaintScreen()
	defer close(scr.release)

	ctx, cancel := context.WithTimeout(context.Background(), window)
	defer cancel()

	// staleSource never bumps its version, so nothing but the repaint ticker
	// can drive a full repaint.
	f := &finder{}
	_, err := f.find(ctx, NewSliceSourceFrom([]string{"alpha"}), Opt{
		Screen:           scr,
		RedrawAggressive: true,
	})
	require.That(t, errors.Is(err, context.DeadlineExceeded), "picker should end on ctx deadline, got %v", err)

	_, syncs := scr.counts()
	assert.That(t, syncs >= 2, "repaints must continue on a still source; got %d in %v", syncs, window)
}
