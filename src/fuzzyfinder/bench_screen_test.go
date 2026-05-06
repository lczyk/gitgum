package fuzzyfinder

// cspell:ignore simscreen serialises

import (
	"sync"

	"github.com/gdamore/tcell/v2"
)

// BenchScreen is a minimal in-memory screen used by benchmarks. it
// satisfies the package-local `screen` interface with a preallocated
// cell buffer so per-frame Clear / SetContent / Show do not allocate.
// closer to a real terminal than tcell.SimulationScreen, which
// allocates inside drawCell + CellBuffer.Fill on every flush.
//
// not goroutine-safe; the finder serialises Show/Sync on its render
// timer and SetContent only happens during draw.
type BenchScreen struct {
	w, h   int
	cells  []benchCell // len == w*h
	cursor struct{ x, y int }

	eventsMu sync.Mutex
	events   []tcell.Event
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

// ChannelEvents drains injected events into ch then returns. the finder
// treats a quiet events channel as the input being idle (real-terminal
// equivalent: nothing typed). bench drives termination via the trailing
// Esc key, same as the simscreen-based tests.
func (s *BenchScreen) ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{}) {
	s.eventsMu.Lock()
	evs := s.events
	s.events = nil
	s.eventsMu.Unlock()
	for _, e := range evs {
		select {
		case ch <- e:
		case <-quit:
			return
		}
	}
}

// SetEvents stages events to be delivered when ChannelEvents runs.
func (s *BenchScreen) SetEvents(events ...tcell.Event) {
	s.eventsMu.Lock()
	s.events = append(s.events, events...)
	s.eventsMu.Unlock()
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
	f.term = bs
	go bs.ChannelEvents(eventsChan, nil)
	return f, bs
}
