package fuzzyfinder

// cspell:ignore simscreen

import (
	"github.com/gdamore/tcell/v2"
)

// BenchScreen is a minimal in-memory screen used by benchmarks. it
// satisfies the package-local `screen` interface with a preallocated
// cell buffer so per-frame Clear / SetContent / Show do not allocate.
// closer to a real terminal than tcell.SimulationScreen, which
// allocates inside drawCell + CellBuffer.Fill on every flush.
//
// not goroutine-safe; the finder is single-reader on the event chan
// and SetContent only happens during draw.
type BenchScreen struct {
	w, h       int
	cells      []benchCell // len == w*h
	cursor     struct{ x, y int }
	eventsChan chan<- tcell.Event
}

type benchCell struct {
	r     rune
	style tcell.Style
}

func newBenchScreen(w, h int) *BenchScreen {
	return &BenchScreen{
		w:     w,
		h:     h,
		cells: make([]benchCell, w*h),
	}
}

func (s *BenchScreen) Init() error      { return nil }
func (s *BenchScreen) Fini()            {}
func (s *BenchScreen) Size() (int, int) { return s.w, s.h }

func (s *BenchScreen) Clear() {
	// zero in place; no realloc.
	for i := range s.cells {
		s.cells[i] = benchCell{}
	}
}

func (s *BenchScreen) SetContent(x, y int, mainc rune, _ []rune, style tcell.Style) {
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		return
	}
	s.cells[y*s.w+x] = benchCell{r: mainc, style: style}
}

func (s *BenchScreen) ShowCursor(x, y int) { s.cursor.x, s.cursor.y = x, y }
func (s *BenchScreen) Show()               {}
func (s *BenchScreen) Sync()               {}

// ChannelEvents satisfies the screen interface. unused on this path
// because NewWithBenchScreen wires SetEvents straight into the finder's
// event chan, so initFinder's `f.term == nil` branch (which is the only
// caller of ChannelEvents) never runs. blocks on quit so the contract
// matches a real screen if anyone ever does call it.
func (s *BenchScreen) ChannelEvents(_ chan<- tcell.Event, quit <-chan struct{}) {
	if quit != nil {
		<-quit
	}
}

// SetEvents pushes events into the finder's event chan. async send so a
// caller staging more events than the chan buffer can hold does not
// deadlock against a finder that hasn't started reading yet.
func (s *BenchScreen) SetEvents(events ...tcell.Event) {
	go func() {
		for _, e := range events {
			s.eventsChan <- e
		}
	}()
}

// NewWithBenchScreen builds a finder wired to a BenchScreen. mirrors
// NewWithMockedTerminal but with the lighter-weight backend, intended
// for benchmarks that want to measure finder allocs rather than mock
// screen allocs.
func NewWithBenchScreen() (*finder, *BenchScreen) {
	eventsChan := make(chan tcell.Event, 32)
	f := &finder{}
	f.termEventsChan = eventsChan
	bs := newBenchScreen(60, 10)
	bs.eventsChan = eventsChan
	f.term = bs
	return f, bs
}
