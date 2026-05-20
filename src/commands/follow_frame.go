package commands

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
)

// followFrame renders the shared chrome for --follow modes: a dim
// `last update: ...` header on row 0, an optional caller-supplied extra
// row (used by `gg diff --follow` for its mode-tab strip), and a body
// region that scrolls / tails / shows errors and paints the right-edge
// truncation indicator.
//
// One frame instance is created per runFollow and reused across redraws;
// it holds the lastMaxOffset needed for tail-resume so callers don't
// have to thread that state themselves.
type followFrame struct {
	scr           *litescreen.Screen
	w, h          int
	headerRows    int
	lastMaxOffset int
}

func newFollowFrame(scr *litescreen.Screen) *followFrame {
	return &followFrame{scr: scr}
}

// Begin starts a fresh redraw: re-reads the screen size and clears the
// surface. Call at the top of every redraw.
func (f *followFrame) Begin() {
	f.w, f.h = f.scr.Size()
	f.scr.Clear()
	f.headerRows = 0
}

// End flushes the redraw to the terminal.
func (f *followFrame) End() {
	f.scr.Show()
}

// Header paints row 0 with the canonical
// `last update: HH:MM:SS -- <interval-text> (<key-hints>)` line in dim.
// intervalText is the part between `--` and the key-hint parens, e.g.
// `2.0s` or `interval 2.0s`. Bumps the header-rows counter so Body
// knows where to start.
func (f *followFrame) Header(interval float64, intervalLabel, keyHints string) {
	dim := tcell.StyleDefault.Dim(true)
	ts := time.Now().Format("15:04:05")
	prefix := fmt.Sprintf("%.1fs", interval)
	if intervalLabel != "" {
		prefix = intervalLabel + " " + prefix
	}
	header := fmt.Sprintf("last update: %s -- %s (%s)", ts, prefix, keyHints)
	writePlain(f.scr, 0, 0, header, dim, f.w, f.h)
	f.headerRows = 1
}

// ExtraRow lets the caller paint an additional top row (mode-specific
// chrome -- e.g. diff's tab strip). The callback receives the row index
// it should paint at. Bumps the header-rows counter.
func (f *followFrame) ExtraRow(paint func(y int)) {
	paint(f.headerRows)
	f.headerRows++
}

// Body paints the visible window of lines starting at the first row
// below the header. Handles the error branch (red one-liner), tail
// mode (clamping scrollOffset to the bottom, auto-resuming tail when
// the user scrolls back past the previous bottom), and the right-edge
// truncation indicator. Mutates *scrollOffset and *tailMode in place.
func (f *followFrame) Body(lines []string, scrollOffset *int, tailMode *bool, cachedErr error) {
	bodyTop := f.headerRows
	visible := max(f.h-bodyTop, 0)

	if cachedErr != nil {
		errStyle := tcell.StyleDefault.Foreground(tcell.PaletteColor(1))
		writePlain(f.scr, 0, bodyTop, "git error: "+cachedErr.Error(), errStyle, f.w, f.h)
		return
	}

	maxOffset := max(len(lines)-visible, 0)
	// tail-resume: if the user scrolled past the prior bottom (e.g. new
	// content arrived while they were at the old bottom), snap back into
	// tail mode so the latest content stays visible.
	if !*tailMode && *scrollOffset >= f.lastMaxOffset {
		*tailMode = true
	}
	if *tailMode {
		*scrollOffset = maxOffset
	}
	*scrollOffset = max(0, min(*scrollOffset, maxOffset))
	f.lastMaxOffset = maxOffset

	end := min(*scrollOffset+visible, len(lines))
	visibleLines := lines[*scrollOffset:end]
	for i, line := range visibleLines {
		writeAnsi(f.scr, 0, bodyTop+i, line, tcell.StyleDefault, f.w, f.h)
	}
	drawTruncIndicator(f.scr, 0, visibleLines, f.w, f.h)
}
