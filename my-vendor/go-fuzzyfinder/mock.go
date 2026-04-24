package fuzzyfinder

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	runewidth "github.com/mattn/go-runewidth"
)

type simScreen tcell.SimulationScreen

// TerminalMock is a mocked terminal for testing.
// Use NewWithMockedTerminal to create one.
type TerminalMock struct {
	simScreen
}

// SetSize changes the pseudo-size of the window.
func (m *TerminalMock) SetSize(w, h int) {
	m.simScreen.SetSize(w, h)
}

// SetEvents sets all events, which are fetched from the terminal event channel.
// A user of this must set the EscKey event at the end.
func (m *TerminalMock) SetEvents(events ...tcell.Event) {
	for _, event := range events {
		switch event := event.(type) {
		case *tcell.EventKey:
			m.simScreen.InjectKey(event.Key(), event.Rune(), event.Modifiers())
		case *tcell.EventResize:
			w, h := event.Size()
			m.simScreen.SetSize(w, h)
		}
	}
}

// GetResult returns a flushed string that is displayed to the actual terminal.
// It contains all escape sequences such that ANSI escape code.
func (m *TerminalMock) GetResult() string {
	var s string

	// set cursor for snapshot test
	setCursor := func() {
		cursorX, cursorY, _ := m.simScreen.GetCursor()
		mainc, _, _, _ := m.simScreen.GetContent(cursorX, cursorY)
		if mainc == ' ' {
			m.simScreen.SetContent(cursorX, cursorY, '█', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDefault))
		} else {
			m.simScreen.SetContent(cursorX, cursorY, mainc, nil, tcell.StyleDefault.Background(tcell.ColorWhite))
		}
		m.simScreen.Show()
	}

	setCursor()

	cells, width, height := m.simScreen.GetContents()

	for h := 0; h < height; h++ {
		prevFg, prevBg := tcell.ColorDefault, tcell.ColorDefault

		for w := 0; w < width; w++ {
			cell := cells[h*width+w]
			fg, bg, attr := cell.Style.Decompose()
			if fg != prevFg || bg != prevBg {
				prevFg, prevBg = fg, bg

				s += "\x1b\x5b\x6d" // Reset previous color.
				v := parseAttr(fg, bg, attr)
				s += v
			}

			s += string(cell.Runes)
			rw := runewidth.RuneWidth(cell.Runes[0])
			if rw != 0 {
				w += rw - 1
			}
		}
		s += "\n"
	}
	s += "\x1b\x5b\x6d" // Reset previous color.

	return s
}

func (f *finder) UseMockedTerminal() *TerminalMock {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		panic(err)
	}
	m := &TerminalMock{
		simScreen: screen,
	}
	f.term = m
	return m
}

// parseAttr parses color and attribute for testing.
func parseAttr(fg, bg tcell.Color, attr tcell.AttrMask) string {
	if attr == tcell.AttrInvalid {
		panic("invalid attribute")
	}

	var params []string
	if attr&tcell.AttrBold == tcell.AttrBold {
		params = append(params, "1")
	}
	if attr&tcell.AttrBlink == tcell.AttrBlink {
		params = append(params, "5")
	}
	if attr&tcell.AttrReverse == tcell.AttrReverse {
		params = append(params, "7")
	}
	if attr&tcell.AttrUnderline == tcell.AttrUnderline {
		params = append(params, "4")
	}
	if attr&tcell.AttrDim == tcell.AttrDim {
		params = append(params, "2")
	}
	if attr&tcell.AttrItalic == tcell.AttrItalic {
		params = append(params, "3")
	}
	if attr&tcell.AttrStrikeThrough == tcell.AttrStrikeThrough {
		params = append(params, "9")
	}

	switch {
	case fg == 0: // Ignore.
	case fg == tcell.ColorDefault:
		params = append(params, "39")
	case fg > tcell.Color255:
		r, g, b := fg.RGB()
		params = append(params, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("38;5;%d", fg-tcell.ColorValid))
	}

	switch {
	case bg == 0: // Ignore.
	case bg == tcell.ColorDefault:
		params = append(params, "49")
	case bg > tcell.Color255:
		r, g, b := bg.RGB()
		params = append(params, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("48;5;%d", bg-tcell.ColorValid))
	}

	return fmt.Sprintf("\x1b[%sm", strings.Join(params, ";"))
}
