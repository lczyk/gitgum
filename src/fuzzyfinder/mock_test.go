package fuzzyfinder

import (
	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
	runewidth "github.com/mattn/go-runewidth"
)

// ansiReset is the ANSI SGR reset sequence (ESC [ m).
const ansiReset = "\x1b[m"

// TerminalMock is a mocked terminal for testing.
// Use NewWithMockedTerminal to create one.
type TerminalMock struct {
	tcell.SimulationScreen
}

// SetEvents sets all events, which are fetched from the terminal event channel.
// A user of this must set the EscKey event at the end.
func (m *TerminalMock) SetEvents(events ...tcell.Event) {
	for _, event := range events {
		switch event := event.(type) {
		case *tcell.EventKey:
			m.InjectKey(event.Key(), event.Rune(), event.Modifiers())
		case *tcell.EventResize:
			m.SetSize(event.Size())
		}
	}
}

// GetResult returns a flushed string that is displayed to the actual terminal.
// It contains all escape sequences such that ANSI escape code.
func (m *TerminalMock) GetResult() string {
	var s string

	// set cursor for snapshot test
	cursorX, cursorY, _ := m.GetCursor()
	mainc, _, _, _ := m.GetContent(cursorX, cursorY)
	if mainc == ' ' {
		m.SetContent(cursorX, cursorY, '█', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDefault))
	} else {
		m.SetContent(cursorX, cursorY, mainc, nil, tcell.StyleDefault.Background(tcell.ColorWhite))
	}
	m.Show()

	cells, width, height := m.GetContents()

	for h := 0; h < height; h++ {
		prevFg, prevBg := tcell.ColorDefault, tcell.ColorDefault

		for w := 0; w < width; w++ {
			cell := cells[h*width+w]
			fg, bg, _ := cell.Style.Decompose()
			if fg != prevFg || bg != prevBg {
				prevFg, prevBg = fg, bg

				s += ansiReset
				if sgr := ansi.StyleToSGR(cell.Style); sgr != "" {
					s += sgr
				} else {
					// historical: emit a bare reset for the default style
					// so transitions back to default produce a visible
					// SGR boundary in golden snapshots.
					s += ansiReset
				}
			}

			// tcell >= v2.7 leaves Runes empty for cells that were never written;
			// treat those as a single space.
			if len(cell.Runes) == 0 {
				s += " "
				continue
			}
			s += string(cell.Runes)
			rw := runewidth.RuneWidth(cell.Runes[0])
			if rw != 0 {
				w += rw - 1
			}
		}
		s += "\n"
	}
	s += ansiReset

	return s
}
