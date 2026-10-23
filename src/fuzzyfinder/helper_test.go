package fuzzyfinder

import "github.com/gdamore/tcell/v2"

func NewWithMockedTerminal() (*finder, *TerminalMock) {
	eventsChan := make(chan tcell.Event, 10)

	f := &finder{}
	f.termEventsChan = eventsChan

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		panic(err)
	}
	m := &TerminalMock{SimulationScreen: screen}
	f.term = m
	go m.ChannelEvents(eventsChan, nil)

	m.SetSize(60, 10)
	return f, m
}

// initItems runs initFinder over items as a feed would hand them over: IDs
// 0..n-1 in order.
func (f *finder) initItems(items []string, opt Opt) error {
	entries := make([]entry, len(items))
	for i, s := range items {
		entries[i] = entry{id: ID(i), text: s}
	}
	return f.initFinder(entries, opt)
}

// newHeadlessFinder returns a finder whose screen draws nowhere and whose
// event channel is preset, so initFinder neither inits a screen nor starts
// an event pump. For tests that drive state directly.
func newHeadlessFinder(multi bool) *finder {
	return &finder{term: initFailScreen{}, termEventsChan: make(chan tcell.Event), multi: multi}
}
