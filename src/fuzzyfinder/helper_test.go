package fuzzyfinder

import "github.com/gdamore/tcell/v2"

func New() *finder {
	return &finder{}
}

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
