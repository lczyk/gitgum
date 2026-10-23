package fuzzyfinder_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)

// frameScreen records the last frame shown and delivers only the keys a test
// presses, so a test can change the feed under an open picker and watch the
// result land before deciding what to press.
type frameScreen struct {
	mu    sync.Mutex
	grid  [10][40]rune
	frame string
	keys  chan tcell.Key
}

func newFrameScreen(t *testing.T) *frameScreen {
	s := &frameScreen{keys: make(chan tcell.Key, 8)}
	t.Cleanup(func() { close(s.keys) })
	return s
}

func (s *frameScreen) press(keys ...tcell.Key) {
	for _, k := range keys {
		s.keys <- k
	}
}

func (s *frameScreen) Init() error      { return nil }
func (s *frameScreen) Fini()            {}
func (s *frameScreen) Size() (int, int) { return 40, 10 }
func (s *frameScreen) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grid = [10][40]rune{}
}

func (s *frameScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if y >= 0 && y < len(s.grid) && x >= 0 && x < len(s.grid[y]) {
		s.grid[y][x] = mainc
	}
}

func (s *frameScreen) ShowCursor(x, y int) {}
func (s *frameScreen) Sync()               { s.Show() }

func (s *frameScreen) Show() {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, row := range s.grid {
		b.WriteString(string(row[:]))
		b.WriteByte('\n')
	}
	s.frame = b.String()
}

func (s *frameScreen) shows(items ...string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		if !strings.Contains(s.frame, it) {
			return false
		}
	}
	return true
}

func (s *frameScreen) hides(item string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frame != "" && !strings.Contains(s.frame, item)
}

func (s *frameScreen) ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{}) {
	for k := range s.keys {
		ch <- tcell.NewEventKey(k, rune(k), tcell.ModNone)
	}
}

func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	assert.Eventually(t, cond, time.Second, 5*time.Millisecond, msg)
}

type findResult struct {
	res ff.Result
	err error
}

func startFind(feed *ff.Feed, opt ff.Opt) <-chan findResult {
	out := make(chan findResult, 1)
	go func() {
		res, err := ff.Find(context.Background(), feed, opt)
		out <- findResult{res, err}
	}()
	return out
}

// Adds, edits and removals made while the picker is open all reach the screen,
// and the cursor follows its item through them.
func TestFind_FeedChangesWhileOpen(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	feed := ff.NewFeed()
	alpha := feed.Add("alpha")
	done := startFind(feed, ff.Opt{Screen: scr})

	beta := feed.Add("beta")
	feed.Update(alpha, "ALPHA")
	eventually(t, func() bool { return scr.shows("ALPHA", "beta") }, "add and edit should both be drawn")

	feed.Remove(alpha)
	eventually(t, func() bool { return scr.hides("ALPHA") }, "removal should be drawn")

	scr.press(tcell.KeyEnter)
	r := <-done
	require.NoError(t, r.err)
	assert.EqualArrays(t, []ff.ID{beta}, r.res.IDs)
	assert.EqualArrays(t, []string{"beta"}, r.res.Items)
}

// A selection follows its item's ID through an edit, and the result reports
// the text as of confirmation.
func TestFind_SelectionSurvivesEdit(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	feed := ff.NewFeedFrom([]string{"one", "two"})
	done := startFind(feed, ff.Opt{Screen: scr, Multi: true})

	scr.press(tcell.KeyTab) // select "one", cursor moves to "two"
	eventually(t, func() bool { return scr.shows(">one") }, "selection should be drawn")
	feed.Update(0, "uno")
	eventually(t, func() bool { return scr.shows(">uno") }, "edit should keep the selection")

	scr.press(tcell.KeyEnter)
	r := <-done
	require.NoError(t, r.err)
	assert.EqualArrays(t, []ff.ID{0}, r.res.IDs)
	assert.EqualArrays(t, []string{"uno"}, r.res.Items)
}

// SelectOne decides once the stream is complete, not on whatever had arrived
// when the picker started. Regression: it checked the first snapshot, so a
// feed seeded with one item auto-selected it before the rest streamed in --
// and a query matching only a later item showed the UI instead.
func TestFind_SelectOneWaitsForClose(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	feed := ff.NewFeed()
	feed.Add("alpha")
	done := startFind(feed, ff.Opt{Screen: scr, SelectOne: true, Query: "beta"})

	beta := feed.Add("beta")
	feed.Close()

	r := <-done
	require.NoError(t, r.err)
	assert.EqualArrays(t, []ff.ID{beta}, r.res.IDs)
	assert.EqualArrays(t, []string{"beta"}, r.res.Items)
}

func TestFind_SelectOneShowsUIWhenSeveralMatch(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	feed := ff.NewFeed()
	feed.Add("alpha")
	done := startFind(feed, ff.Opt{Screen: scr, SelectOne: true})
	feed.Add("beta")
	feed.Close()

	eventually(t, func() bool { return scr.shows("alpha", "beta") }, "picker should open with both items")
	scr.press(tcell.KeyEnter)
	r := <-done
	require.NoError(t, r.err)
	assert.EqualArrays(t, []string{"alpha"}, r.res.Items)
}

func TestFind_SelectOneHonoursCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ff.Find(ctx, ff.NewFeed(), ff.Opt{SelectOne: true})
	assert.Error(t, err, context.Canceled)
}

const spinnerRunes = "\u280b\u2819\u2839\u2838\u283c\u2834\u2826\u2827\u2807\u280f"

func (s *frameScreen) spinning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.ContainsAny(s.frame, spinnerRunes)
}

// An open feed shows a spinner by the prompt; closing it stops the spinner.
func TestFind_SpinnerUntilClose(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	feed := ff.NewFeed()
	feed.Add("alpha")
	done := startFind(feed, ff.Opt{Screen: scr})

	eventually(t, scr.spinning, "an open feed should show the spinner")
	feed.Close()
	eventually(t, func() bool { return scr.shows("alpha") && !scr.spinning() }, "closing should stop the spinner")

	scr.press(tcell.KeyEnter)
	r := <-done
	require.NoError(t, r.err)
}

// A closed feed never shows the spinner.
func TestFind_NoSpinnerForClosedFeed(t *testing.T) {
	t.Parallel()

	scr := newFrameScreen(t)
	done := startFind(ff.NewFeedFrom([]string{"alpha"}), ff.Opt{Screen: scr})

	eventually(t, func() bool { return scr.shows("alpha") }, "first frame")
	assert.That(t, !scr.spinning(), "closed feed should not spin")

	scr.press(tcell.KeyEnter)
	r := <-done
	require.NoError(t, r.err)
}
