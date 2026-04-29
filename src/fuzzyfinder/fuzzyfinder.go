// Package fuzzyfinder provides terminal user interfaces for fuzzy-finding.
//
// Note that, all functions are not goroutine-safe.
package fuzzyfinder

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
	"github.com/lczyk/gitgum/src/litescreen"
	runewidth "github.com/mattn/go-runewidth"
)

// newScreen returns the screen backend. Default is litescreen (our own
// ANSI renderer) which supports both fullscreen and inline (Opt.Height)
// modes. Set FF_RENDERER=legacy to fall back to tcell's alt-screen
// renderer; tcell can't preserve scrollback so Opt.Height is ignored in
// that case.
func newScreen(height int) (screen, error) {
	if os.Getenv("FF_RENDERER") == "legacy" {
		return tcell.NewScreen()
	}
	return litescreen.New(height)
}

var (
	// ErrAbort is returned from Find* functions if there are no selections.
	ErrAbort   = errors.New("abort")
	errEntered = errors.New("entered")
)

type state struct {
	items      []string // All item names.
	itemsLower []string // Lowercased view of items for matching's hot path.
	matched    []int    // Matched items against the input.

	// x is the current index of the prompt line.
	x int
	// cursorX is the position of prompt line.
	// Note that cursorX is the actual width of input runes.
	cursorX int

	// The current index of filtered items (matched).
	// The initial value is 0.
	y int
	// cursorY is the position of item line.
	// Note that the max size of cursorY depends on max height.
	cursorY int

	input []rune

	// selection holds the multi-select state. key = items index, value = selection order (1-based).
	selection map[int]int
	// selectionIdx holds the next index, which is used to a selection's value.
	selectionIdx int
}

// screen is the subset of tcell.Screen the finder actually uses. Pulling it
// out as a local interface lets us swap in a non-tcell renderer later (e.g.
// an inline ANSI renderer that preserves terminal scrollback) without
// touching the finder logic. tcell.Screen and tcell.SimulationScreen both
// satisfy it implicitly.
type screen interface {
	Init() error
	Fini()
	Size() (int, int)
	Clear()
	SetContent(x, y int, mainc rune, combc []rune, style tcell.Style)
	ShowCursor(x, y int)
	Show()
	// Sync forces a full repaint. Used to recover from a sibling process
	// writing to the terminal mid-render (e.g. find's stderr "permission
	// denied" lines tearing the picker). tcell.Screen and litescreen.Screen
	// both provide this; tcell.SimulationScreen inherits it via embedding
	// in TerminalMock.
	Sync()
	ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{})
}

type finder struct {
	term      screen
	stateMu   sync.RWMutex
	state     state
	drawTimer *time.Timer
	eventCh   chan struct{}
	opt       *Opt
	multi     bool

	termEventsChan <-chan tcell.Event
}

// chromeRows returns the count of non-item rows the picker draws around the
// item area: the prompt+number-line row (merged), plus header when set. Used
// to translate Opt.Height (item-row count) into total framebuf rows.
func chromeRows(opt Opt) int {
	n := 1 // merged prompt + number-line
	if opt.Header != "" {
		n++
	}
	return n
}

func (f *finder) initFinder(items []string, opt Opt) error {
	if f.term == nil {
		screenH := opt.Height
		if screenH > 0 {
			screenH += chromeRows(opt)
		}
		s, err := newScreen(screenH)
		if err != nil {
			return fmt.Errorf("failed to new screen: %w", err)
		}
		f.term = s
		if err := f.term.Init(); err != nil {
			return fmt.Errorf("failed to initialize screen: %w", err)
		}

		eventsChan := make(chan tcell.Event)
		go f.term.ChannelEvents(eventsChan, nil)
		f.termEventsChan = eventsChan
	}

	f.opt = &opt
	f.state = state{}

	if f.multi {
		f.state.selection = map[int]int{}
		f.state.selectionIdx = 1
	}

	f.state.items = items
	f.state.itemsLower = lowerHaystack(nil, nil, items)
	f.state.matched = makeMatched(len(items))

	if !isInTesting() {
		f.drawTimer = time.AfterFunc(0, func() {
			f.stateMu.Lock()
			f._draw()
			f.stateMu.Unlock()
			f.flush()
		})
		f.drawTimer.Stop()
	}
	f.eventCh = make(chan struct{}, 30) // A large value

	if opt.Query != "" {
		f.state.input = []rune(opt.Query)
		f.state.cursorX = runewidth.StringWidth(opt.Query)
		f.state.x = len(opt.Query)
		f.filter()
	}

	return nil
}

// updateItems atomically replaces the items slice and re-derives matched,
// cursor, and selection so they keep pointing at the same logical items where
// possible. Items that have been removed drop out of the selection; the
// cursor follows its item if it still exists, otherwise it clamps.
func (f *finder) updateItems(items []string) {
	f.stateMu.Lock()

	// Capture identity keys against the OLD items before replacing.
	cursorKey, cursorValid := f.cursorIdentityLocked()
	selKeys, selOrders := f.selectionIdentitiesLocked()

	prevItems := f.state.items
	f.state.items = items
	f.state.itemsLower = lowerHaystack(f.state.itemsLower, prevItems, items)

	// Recompute matched against current input. Mirrors filter() but assumes
	// the lock is held — callers from the resync goroutine want one atomic
	// transition without dropping the lock mid-way.
	if len(f.state.input) == 0 {
		f.resetMatchedIdentity(len(items))
	} else {
		f.state.matched = matching.FindAllLower(strings.ToLower(string(f.state.input)), f.state.itemsLower)
	}

	// Re-key selection: drop entries whose item is gone; keep selection order
	// for survivors.
	if len(selKeys) > 0 {
		newSel := make(map[int]int, len(selKeys))
		for i, k := range selKeys {
			if newIdx, ok := findKey(items, k); ok {
				newSel[newIdx] = selOrders[i]
			}
		}
		f.state.selection = newSel
	}

	// Re-key cursor: if the cursored item still exists *and* still matches
	// the query, point at its new position in matched. Otherwise clamp.
	f.remapCursorLocked(cursorKey, cursorValid)

	f.stateMu.Unlock()

	// Non-blocking signal: trigger redraw. The eventCh consumer also calls
	// filter() but that's idempotent against the steady state we just wrote.
	select {
	case f.eventCh <- struct{}{}:
	default:
	}
}

// cursorIdentityLocked returns the identity key of the cursored item.
// Caller must hold f.stateMu.
func (f *finder) cursorIdentityLocked() (itemKey, bool) {
	if len(f.state.matched) == 0 {
		return itemKey{}, false
	}
	if f.state.y < 0 || f.state.y >= len(f.state.matched) {
		return itemKey{}, false
	}
	idx := f.state.matched[f.state.y]
	if idx < 0 || idx >= len(f.state.items) {
		return itemKey{}, false
	}
	return keyOf(f.state.items, idx), true
}

// selectionIdentitiesLocked returns identity keys + selection orders for
// every selected item. Order arrays line up by index.
// Caller must hold f.stateMu.
func (f *finder) selectionIdentitiesLocked() ([]itemKey, []int) {
	if len(f.state.selection) == 0 {
		return nil, nil
	}
	keys := make([]itemKey, 0, len(f.state.selection))
	orders := make([]int, 0, len(f.state.selection))
	for idx, ord := range f.state.selection {
		if idx < 0 || idx >= len(f.state.items) {
			continue
		}
		keys = append(keys, keyOf(f.state.items, idx))
		orders = append(orders, ord)
	}
	return keys, orders
}

// remapCursorLocked points state.y at the new position of the cursored item
// in the new matched list. If the item is gone or no longer matches the
// query, clamps to a valid value.
// Caller must hold f.stateMu.
func (f *finder) remapCursorLocked(key itemKey, valid bool) {
	if !valid {
		f.clampCursorLocked()
		return
	}
	newIdx, ok := findKey(f.state.items, key)
	if !ok {
		f.clampCursorLocked()
		return
	}
	for y, m := range f.state.matched {
		if m == newIdx {
			f.state.y = y
			if f.state.cursorY > y {
				f.state.cursorY = y
			}
			return
		}
	}
	f.clampCursorLocked()
}

// clampCursorLocked ensures state.y / state.cursorY are within matched bounds.
// Caller must hold f.stateMu.
func (f *finder) clampCursorLocked() {
	n := len(f.state.matched)
	if n == 0 {
		f.state.y = 0
		f.state.cursorY = 0
		return
	}
	if f.state.y >= n {
		f.state.y = n - 1
	}
	if f.state.cursorY > f.state.y {
		f.state.cursorY = f.state.y
	}
	if f.state.y < 0 {
		f.state.y = 0
	}
	if f.state.cursorY < 0 {
		f.state.cursorY = 0
	}
}

// _draw is used from draw with a timer.
func (f *finder) _draw() {
	width, height := f.term.Size()
	f.term.Clear()

	maxWidth := width

	// Layout: rows are addressed as offsets from the prompt outward into the
	// item area. step is +1 in reverse mode (prompt at top, items grow down)
	// and -1 otherwise (prompt at bottom, items grow up).
	step := -1
	promptRow := height - 1
	if f.opt.Reverse {
		step = 1
		promptRow = 0
	}
	rowAt := func(offset int) int { return promptRow + step*offset }

	// Header line (offset 1 from prompt) — only present when set.
	offset := 1
	var w int
	if len(f.opt.Header) > 0 {
		for _, r := range runewidth.Truncate(f.opt.Header, maxWidth-2, "..") {
			style := tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorDefault)
			f.term.SetContent(2+w, rowAt(offset), r, nil, style)
			w += runewidth.RuneWidth(r)
		}
		offset++
	}

	// Item area sizing. Number-line shares the prompt row, so chrome above
	// the items is just `offset` rows (prompt + optional header).
	firstItemOffset := offset
	itemAreaHeight := height - firstItemOffset - 1
	if itemAreaHeight < 0 {
		itemAreaHeight = 0
	}
	pageSize := itemAreaHeight + 1
	topIdx := f.state.y - f.state.cursorY

	// Number-line: counts + page indicator, drawn at the left of the prompt
	// row; the prompt and query render to its right separated by a small gap.
	// Numerators are left-padded with grey '0's (and the page section is
	// always shown) so the rendered width stays constant as the user types —
	// the prompt column doesn't jump around.
	totalItems := len(f.state.items)
	matchedCount := len(f.state.matched)
	tw := len(fmt.Sprintf("%d", totalItems))
	page, totalPages := 1, 1
	maxPages := 1
	if pageSize > 0 {
		page = f.state.y/pageSize + 1
		if matchedCount > 0 {
			totalPages = (matchedCount + pageSize - 1) / pageSize
		}
		maxPages = (totalItems + pageSize - 1) / pageSize
		if maxPages < 1 {
			maxPages = 1
		}
	}
	pw := len(fmt.Sprintf("%d", maxPages))

	yellow := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
	grey := tcell.StyleDefault.Foreground(tcell.ColorDarkGray).Background(tcell.ColorDefault)
	col := 0
	emit := func(s string, st tcell.Style) {
		for _, r := range s {
			f.term.SetContent(col, promptRow, r, nil, st)
			col++
		}
	}
	emitPadded := func(n, w int) {
		s := fmt.Sprintf("%d", n)
		if pad := w - len(s); pad > 0 {
			emit(strings.Repeat("0", pad), grey)
		}
		emit(s, yellow)
	}
	emitPadded(matchedCount, tw)
	emit("/", yellow)
	emit(fmt.Sprintf("%d", totalItems), yellow)
	emit("  page ", yellow)
	emitPadded(page, pw)
	emit("/", yellow)
	emitPadded(totalPages, pw)

	const numberPromptGap = 2
	promptCol := col + numberPromptGap
	for _, r := range f.opt.Prompt {
		style := tcell.StyleDefault.Foreground(tcell.ColorBlue).Background(tcell.ColorDefault)
		f.term.SetContent(promptCol, promptRow, r, nil, style)
		promptCol++
	}
	w = 0
	for _, r := range f.state.input {
		style := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault).Bold(true)
		f.term.SetContent(promptCol+w, promptRow, r, nil, style)
		w += runewidth.RuneWidth(r)
	}
	f.term.ShowCursor(promptCol+f.state.cursorX, promptRow)

	// Item lines: i=0 is the row closest to the prompt.
	matched := f.state.matched[topIdx:]
	words := strings.Fields(string(f.state.input))

	for i, m := range matched {
		if i > itemAreaHeight {
			break
		}
		row := rowAt(firstItemOffset + i)
		if i == f.state.cursorY {
			style := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlack)
			f.term.SetContent(0, row, '>', nil, style)
			f.term.SetContent(1, row, ' ', nil, style)
		}

		if f.multi {
			if _, ok := f.state.selection[m]; ok {
				style := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlack)
				f.term.SetContent(1, row, '>', nil, style)
			}
		}

		// Compute positions to highlight for multi-word matching
		var highlightPositions map[int]bool
		itemRunes := []rune(f.state.items[m])
		lowerItemRunes := []rune(strings.ToLower(f.state.items[m]))
		for _, word := range words {
			lowerWord := strings.ToLower(word)
			wordRunes := []rune(lowerWord)
			for i := 0; i <= len(lowerItemRunes)-len(wordRunes); i++ {
				match := true
				for k := 0; k < len(wordRunes); k++ {
					if lowerItemRunes[i+k] != wordRunes[k] {
						match = false
						break
					}
				}
				if match {
					if highlightPositions == nil {
						highlightPositions = make(map[int]bool)
					}
					for k := 0; k < len(wordRunes); k++ {
						highlightPositions[i+k] = true
					}
				}
			}
		}

		w := 2
		for j, r := range itemRunes {
			style := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault)
			hasHighlighted := highlightPositions[j]
			if hasHighlighted {
				style = tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorDefault)
			}
			if i == f.state.cursorY {
				if hasHighlighted {
					style = tcell.StyleDefault.Foreground(tcell.ColorDarkCyan).Bold(true).Background(tcell.ColorBlack)
				} else {
					style = tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true).Background(tcell.ColorBlack)
				}
			}

			rw := runewidth.RuneWidth(r)
			if w+rw+2 > maxWidth {
				f.term.SetContent(w, row, '.', nil, style)
				f.term.SetContent(w+1, row, '.', nil, style)
				break
			}
			f.term.SetContent(w, row, r, nil, style)
			w += rw
		}
	}

	// Scrollbar in the rightmost column of the item area, only when the
	// matched list overflows the visible window.
	if pageSize > 0 && len(f.state.matched) > pageSize {
		trackHeight := pageSize
		total := len(f.state.matched)

		thumbSize := max(1, trackHeight*pageSize/total)
		if thumbSize > trackHeight {
			thumbSize = trackHeight
		}
		visibleFar := topIdx + min(itemAreaHeight, total-1-topIdx)
		maxOffset := trackHeight - thumbSize
		thumbOff := 0
		if total > pageSize {
			// In bottom-up mode, thumb at the top track row corresponds to
			// the worst-rank items (highest matched index). In reverse mode,
			// the track row closest to the prompt is the best item.
			thumbOff = (total - 1 - visibleFar) * maxOffset / (total - pageSize)
		}
		thumbOff = min(maxOffset, max(0, thumbOff))

		trackStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkGray).Background(tcell.ColorDefault)
		thumbStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
		col := maxWidth - 1
		for row := 0; row < trackHeight; row++ {
			// Track row 0 is the row farthest from the prompt; row trackHeight-1
			// is closest. firstItemOffset+itemAreaHeight is the far end.
			y := rowAt(firstItemOffset + (trackHeight - 1 - row))
			if row >= thumbOff && row < thumbOff+thumbSize {
				f.term.SetContent(col, y, '█', nil, thumbStyle)
			} else {
				f.term.SetContent(col, y, '│', nil, trackStyle)
			}
		}
	}
}

func (f *finder) draw(d time.Duration) {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()

	if isInTesting() {
		// Don't use goroutine scheduling.
		f._draw()
		f.flush()
	} else {
		f.drawTimer.Reset(d)
	}
}

// flush emits the current frame. Picks Sync (full repaint) when the caller
// asked for aggressive redraws — typically because a sibling process is
// writing to the same terminal — otherwise the cheaper diff-based Show.
func (f *finder) flush() {
	if f.opt != nil && f.opt.RedrawAggressive {
		f.term.Sync()
		return
	}
	f.term.Show()
}

// readKey reads a key input.
// It returns ErrAbort if esc, CTRL-C or CTRL-D keys are inputted,
// errEntered in case of enter key, and a context error when the passed
// context is cancelled.
func (f *finder) readKey(ctx context.Context) error {
	f.stateMu.RLock()
	prevInputLen := len(f.state.input)
	f.stateMu.RUnlock()
	defer func() {
		f.stateMu.RLock()
		currentInputLen := len(f.state.input)
		f.stateMu.RUnlock()
		if prevInputLen != currentInputLen {
			f.eventCh <- struct{}{}
		}
	}()

	var e tcell.Event

	select {
	case ee := <-f.termEventsChan:
		e = ee
	case <-ctx.Done():
		return ctx.Err()
	}

	f.stateMu.Lock()
	defer f.stateMu.Unlock()

	_, screenHeight := f.term.Size()
	matchedLinesCount := len(f.state.matched)

	// Visible item-row count, must match _draw's pageSize so Ctrl+B/F align
	// to the same boundaries as the renderer paints. Number-line shares the
	// prompt row, so chrome above items = 1 + (header ? 1 : 0).
	firstItemOffset := 1
	if len(f.opt.Header) > 0 {
		firstItemOffset = 2
	}
	pageSize := screenHeight - firstItemOffset
	if pageSize < 1 {
		pageSize = 1
	}

	switch e := e.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyEsc, tcell.KeyCtrlC, tcell.KeyCtrlD:
			return ErrAbort
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(f.state.input) == 0 {
				return nil
			}
			if f.state.x == 0 {
				return nil
			}
			x := f.state.x
			f.state.cursorX -= runewidth.RuneWidth(f.state.input[x-1])
			f.state.x--
			f.state.input = append(f.state.input[:x-1], f.state.input[x:]...)
		case tcell.KeyDelete:
			if f.state.x == len(f.state.input) {
				return nil
			}
			x := f.state.x

			f.state.input = append(f.state.input[:x], f.state.input[x+1:]...)
		case tcell.KeyEnter:
			return errEntered
		case tcell.KeyLeft:
			if f.state.x > 0 {
				f.state.cursorX -= runewidth.RuneWidth(f.state.input[f.state.x-1])
				f.state.x--
			}
		case tcell.KeyRight:
			if f.state.x < len(f.state.input) {
				f.state.cursorX += runewidth.RuneWidth(f.state.input[f.state.x])
				f.state.x++
			}
		case tcell.KeyCtrlA, tcell.KeyHome:
			f.state.cursorX = 0
			f.state.x = 0
		case tcell.KeyCtrlE, tcell.KeyEnd:
			f.state.cursorX = runewidth.StringWidth(string(f.state.input))
			f.state.x = len(f.state.input)
		case tcell.KeyCtrlW:
			in := f.state.input[:f.state.x]
			inStr := string(in)
			pos := strings.LastIndex(strings.TrimRightFunc(inStr, unicode.IsSpace), " ")
			if pos == -1 {
				f.state.input = []rune{}
				f.state.cursorX = 0
				f.state.x = 0
				return nil
			}
			pos = utf8.RuneCountInString(inStr[:pos])
			newIn := f.state.input[:pos+1]
			f.state.input = newIn
			f.state.cursorX = runewidth.StringWidth(string(newIn))
			f.state.x = len(newIn)
		case tcell.KeyCtrlU:
			f.state.input = f.state.input[f.state.x:]
			f.state.cursorX = 0
			f.state.x = 0
		case tcell.KeyUp, tcell.KeyCtrlK, tcell.KeyCtrlP:
			// Visually upward. In bottom-up layout that means away from the
			// bottom prompt; in reverse layout it means toward the top prompt.
			// Ctrl+Up acts as PgUp (page jump).
			if e.Modifiers()&tcell.ModCtrl != 0 && e.Key() == tcell.KeyUp {
				if f.opt.Reverse {
					f.pageTowardPrompt(pageSize, matchedLinesCount)
				} else {
					f.pageAwayFromPrompt(pageSize, matchedLinesCount)
				}
			} else if f.opt.Reverse {
				f.scrollTowardPrompt(pageSize, matchedLinesCount)
			} else {
				f.scrollAwayFromPrompt(pageSize, matchedLinesCount)
			}
		case tcell.KeyDown, tcell.KeyCtrlJ, tcell.KeyCtrlN:
			// Ctrl+Down acts as PgDn (page jump).
			if e.Modifiers()&tcell.ModCtrl != 0 && e.Key() == tcell.KeyDown {
				if f.opt.Reverse {
					f.pageAwayFromPrompt(pageSize, matchedLinesCount)
				} else {
					f.pageTowardPrompt(pageSize, matchedLinesCount)
				}
			} else if f.opt.Reverse {
				f.scrollAwayFromPrompt(pageSize, matchedLinesCount)
			} else {
				f.scrollTowardPrompt(pageSize, matchedLinesCount)
			}
		case tcell.KeyPgUp, tcell.KeyCtrlB:
			if f.opt.Reverse {
				f.pageTowardPrompt(pageSize, matchedLinesCount)
			} else {
				f.pageAwayFromPrompt(pageSize, matchedLinesCount)
			}
		case tcell.KeyPgDn, tcell.KeyCtrlF:
			if f.opt.Reverse {
				f.pageAwayFromPrompt(pageSize, matchedLinesCount)
			} else {
				f.pageTowardPrompt(pageSize, matchedLinesCount)
			}
		case tcell.KeyTab:
			if !f.multi {
				return nil
			}
			idx := f.state.matched[f.state.y]
			if _, ok := f.state.selection[idx]; ok {
				delete(f.state.selection, idx)
			} else {
				f.state.selection[idx] = f.state.selectionIdx
				f.state.selectionIdx++
			}
			f.scrollAwayFromPrompt(pageSize, matchedLinesCount)
		default:
			if e.Rune() != 0 {
				width, _ := f.term.Size()
				maxLineWidth := width - 2 - 1
				if len(f.state.input)+1 > maxLineWidth {
					// Discard inputted rune.
					return nil
				}

				x := f.state.x
				f.state.input = append(f.state.input[:x], append([]rune{e.Rune()}, f.state.input[x:]...)...)
				f.state.cursorX += runewidth.RuneWidth(e.Rune())
				f.state.x++
			}
		}
	case *tcell.EventResize:
		f.term.Clear()

		width, height := f.term.Size()
		// Recompute cursorY for the new page size (strict alignment).
		newFirstItemOffset := 2
		if len(f.opt.Header) > 0 {
			newFirstItemOffset = 3
		}
		newPageSize := height - newFirstItemOffset
		if newPageSize > 0 {
			f.state.cursorY = f.state.y % newPageSize
		}

		maxLineWidth := width - 2 - 1
		if maxLineWidth < 0 {
			f.state.input = nil
			f.state.cursorX = 0
			f.state.x = 0
		} else if len(f.state.input)+1 > maxLineWidth {
			// Discard inputted rune.
			f.state.input = f.state.input[:maxLineWidth]
			f.state.cursorX = runewidth.StringWidth(string(f.state.input))
			f.state.x = maxLineWidth
		}
	}
	return nil
}

// Cursor scroll helpers. "Toward prompt" moves toward the best match (state.y
// decreases); "away from prompt" moves toward worse matches (state.y
// increases). Both wrap when at the boundary.

// Strict-page navigation. Visible items are always page-aligned: topIdx =
// (y/pageSize)*pageSize, cursorY = y%pageSize. Crossing a page boundary
// shifts the visible page; mid-page moves leave the page unchanged.

// scrollAwayFromPrompt advances y by 1 (forward in matched), wrapping past
// the end. cursorY recomputed from y%pageSize so crossing a page boundary
// shifts the visible page.
func (f *finder) scrollAwayFromPrompt(pageSize, matchedLinesCount int) {
	if matchedLinesCount == 0 {
		return
	}
	f.state.y = (f.state.y + 1) % matchedLinesCount
	f.state.cursorY = f.state.y % pageSize
}

// scrollTowardPrompt is the symmetric step backward.
func (f *finder) scrollTowardPrompt(pageSize, matchedLinesCount int) {
	if matchedLinesCount == 0 {
		return
	}
	f.state.y = ((f.state.y-1)%matchedLinesCount + matchedLinesCount) % matchedLinesCount
	f.state.cursorY = f.state.y % pageSize
}

// pageAwayFromPrompt jumps to the same in-page offset on the next page,
// wrapping to page 0 past the last page. The cursor stays at the same
// visual row; only the visible page changes.
func (f *finder) pageAwayFromPrompt(pageSize, matchedLinesCount int) {
	if matchedLinesCount == 0 {
		return
	}
	totalPages := (matchedLinesCount + pageSize - 1) / pageSize
	currentPage := f.state.y / pageSize
	newPage := (currentPage + 1) % totalPages
	newY := newPage*pageSize + f.state.cursorY
	if newY >= matchedLinesCount {
		// last page short of cursorY rows: clamp to last item
		newY = matchedLinesCount - 1
	}
	f.state.y = newY
	f.state.cursorY = newY % pageSize
}

// pageTowardPrompt is the symmetric jump backward, cycling to the last
// page when stepping back past page 0.
func (f *finder) pageTowardPrompt(pageSize, matchedLinesCount int) {
	if matchedLinesCount == 0 {
		return
	}
	totalPages := (matchedLinesCount + pageSize - 1) / pageSize
	currentPage := f.state.y / pageSize
	newPage := (currentPage - 1 + totalPages) % totalPages
	newY := newPage*pageSize + f.state.cursorY
	if newY >= matchedLinesCount {
		newY = matchedLinesCount - 1
	}
	f.state.y = newY
	f.state.cursorY = newY % pageSize
}

func (f *finder) filter() {
	f.stateMu.RLock()
	if len(f.state.input) == 0 {
		f.stateMu.RUnlock()
		f.stateMu.Lock()
		defer f.stateMu.Unlock()
		f.state.matched = makeMatched(len(f.state.items))
		return
	}

	// FindAll may take a lot of time, so it is desired to use RLock to avoid goroutine blocking.
	matchedItems := matching.FindAllLower(strings.ToLower(string(f.state.input)), f.state.itemsLower)
	f.stateMu.RUnlock()

	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.state.matched = matchedItems
	if len(f.state.matched) == 0 {
		f.state.cursorY = 0
		f.state.y = 0
		return
	}

	switch {
	case f.state.cursorY >= len(f.state.matched):
		f.state.cursorY = len(f.state.matched) - 1
		f.state.y = len(f.state.matched) - 1
	case f.state.y >= len(f.state.matched):
		f.state.y = len(f.state.matched) - 1
	}
}

// find runs the picker against a Source. The picker takes an initial snapshot,
// then a background goroutine polls Version (if implemented) on a 30ms cadence
// and re-snapshots when it changes. Sources without Version always re-snapshot
// each tick.
func (f *finder) find(ctx context.Context, src Source, opt Opt) ([]int, error) {
	if src == nil {
		return nil, errors.New("source must not be nil")
	}

	opt = opt.withDefaults()
	f.multi = opt.Multi

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	versioned, _ := src.(Versioned)
	var lastVersion uint64
	if versioned != nil {
		lastVersion = versioned.Version()
	}
	initial := src.Snapshot()

	initialized := make(chan struct{})
	go func() {
		<-initialized
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if versioned != nil {
					v := versioned.Version()
					if v == lastVersion {
						// No source change. In aggressive mode also poke
						// the redraw channel so the picker repaints over
						// any tearing from a sibling writing to the
						// terminal between source updates.
						if opt.RedrawAggressive {
							select {
							case f.eventCh <- struct{}{}:
							default:
							}
						}
						continue
					}
					lastVersion = v
				}
				f.updateItems(src.Snapshot())
			}
		}
	}()

	if err := f.initFinder(initial, opt); err != nil {
		return nil, fmt.Errorf("failed to initialize the fuzzy finder: %w", err)
	}
	close(initialized)
	return f.runLoop(ctx, &opt)
}

func makeMatched(n int) []int {
	matched := make([]int, n)
	for i := range matched {
		matched[i] = i
	}
	return matched
}

// lowerHaystack updates dst so dst[i] == strings.ToLower(items[i]) for all i.
// Reuses dst when its capacity allows and only re-lowers entries that differ
// from prevItems (the previously-cached source). Pass prevItems=nil to force
// a full rebuild. ASCII-lowercase items get aliased (Go's strings.ToLower
// returns the input string when no rune needs lowering), so a fully-lowercase
// haystack costs ~zero extra memory.
func lowerHaystack(dst, prevItems, items []string) []string {
	if cap(dst) >= len(items) {
		dst = dst[:len(items)]
	} else {
		newDst := make([]string, len(items))
		copy(newDst, dst)
		dst = newDst
	}
	for i, s := range items {
		if i < len(prevItems) && prevItems[i] == s {
			continue // dst[i] already holds the right lowercase form
		}
		dst[i] = strings.ToLower(s)
	}
	return dst
}

// resetMatchedIdentity rewrites f.state.matched in place to [0..n), reusing
// the existing backing array when its capacity allows. This is the hot path
// when the picker has no query — every resync rebuilds an identity-mapping
// matched list, and at n=10k the allocation alone was ~80 KB per call.
func (f *finder) resetMatchedIdentity(n int) {
	if cap(f.state.matched) >= n {
		f.state.matched = f.state.matched[:n]
	} else {
		f.state.matched = make([]int, n)
	}
	for i := range f.state.matched {
		f.state.matched[i] = i
	}
}

func (f *finder) runLoop(ctx context.Context, opt *Opt) ([]int, error) {
	if !isInTesting() {
		defer f.term.Fini()
	}

	if opt.SelectOne && len(f.state.matched) == 1 {
		return []int{f.state.matched[0]}, nil
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-f.eventCh:
				f.filter()
				f.draw(0)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			f.draw(10 * time.Millisecond)

			err := f.readKey(ctx)
			// hack for earning time to filter exec
			if isInTesting() {
				time.Sleep(50 * time.Millisecond)
			}
			switch {
			case errors.Is(err, ErrAbort):
				return nil, ErrAbort
			case errors.Is(err, errEntered):
				f.stateMu.RLock()
				defer f.stateMu.RUnlock()

				if len(f.state.matched) == 0 {
					return nil, ErrAbort
				}
				if f.multi {
					if len(f.state.selection) == 0 {
						return []int{f.state.matched[f.state.y]}, nil
					}
					poss, idxs := make([]int, 0, len(f.state.selection)), make([]int, 0, len(f.state.selection))
					for idx, pos := range f.state.selection {
						idxs = append(idxs, idx)
						poss = append(poss, pos)
					}
					sort.Slice(idxs, func(i, j int) bool {
						return poss[i] < poss[j]
					})
					return idxs, nil
				}
				return []int{f.state.matched[f.state.y]}, nil
			case err != nil:
				return nil, fmt.Errorf("failed to read a key: %w", err)
			}
		}
	}
}

// Find displays a fuzzy-finder UI over items and returns the indices of the
// selected entries, or ErrAbort if the user cancels. With Opt.Multi=false,
// the returned slice always has exactly one element.
//
// Pass lock=nil for a static slice. Pass a non-nil lock when the slice may
// grow concurrently — the picker re-snapshots under lock on a 30ms cadence.
// Length-equal mutations (e.g. in-place edits or balanced add+remove) are not
// detected on this path; for that, use FindFromSource with a SliceSource.
func Find(ctx context.Context, items *[]string, lock sync.Locker, opt Opt) ([]int, error) {
	if items == nil {
		return nil, errors.New("items pointer must not be nil")
	}
	f := &finder{}
	return f.Find(ctx, items, lock, opt)
}

func (f *finder) Find(ctx context.Context, items *[]string, lock sync.Locker, opt Opt) ([]int, error) {
	return f.find(ctx, &legacyLockedSource{items: items, lock: lock}, opt)
}

// FindFromSource displays the picker over a Source and returns the selected
// items as strings, or ErrAbort if the user cancels. With Opt.Multi=false the
// returned slice always has exactly one element.
//
// Unlike Find, FindFromSource supports both adding and removing items while
// the picker is open: callers mutate the source via SliceSource (or any
// custom Source) and the picker resyncs on the next 30ms tick. Cursor and
// selection are preserved across resyncs by item identity, not slice index.
func FindFromSource(ctx context.Context, src Source, opt Opt) ([]string, error) {
	f := &finder{}
	return f.FindFromSource(ctx, src, opt)
}

// FindFromSource is the picker-method form, used by tests that need to inject
// a mocked terminal.
func (f *finder) FindFromSource(ctx context.Context, src Source, opt Opt) ([]string, error) {
	idxs, err := f.find(ctx, src, opt)
	if err != nil {
		return nil, err
	}
	return f.itemsAtLocked(idxs), nil
}

// itemsAtLocked translates indices into the picker's terminal items snapshot.
// Out-of-range indices are dropped.
func (f *finder) itemsAtLocked(idxs []int) []string {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	out := make([]string, 0, len(idxs))
	for _, i := range idxs {
		if i >= 0 && i < len(f.state.items) {
			out = append(out, f.state.items[i])
		}
	}
	return out
}

func isInTesting() bool {
	return flag.Lookup("test.v") != nil
}
