// Package fuzzyfinder provides terminal user interfaces for fuzzy-finding.
//
// Feed is goroutine-safe; a finder run is not.
package fuzzyfinder

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
	"github.com/lczyk/gitgum/src/litescreen"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
	runewidth "github.com/mattn/go-runewidth"
)

var (
	// ErrAbort is returned from Find* functions when the user cancels the
	// picker (Esc, Ctrl-C, Ctrl-D).
	ErrAbort   = errors.New("abort")
	errEntered = errors.New("entered")
)

// Result is what a Find* run produces. IDs and Items describe the same
// selection: IDs the Feed handles, Items the text each had at confirmation
// (ANSI-stripped when Opt.Ansi). Query is always the query as typed when the
// picker exited, whether or not anything was selected.
//
// A confirmed selection yields a non-empty IDs/Items. Pressing Enter when
// no matched item is selectable -- no matches at all, or every match ruled out
// by Opt.Unselectable -- ends the picker with both empty and a nil error: the
// picker cannot produce a selection, so it doesn't pretend to, and the caller
// decides what the bare Query means. Cancelling returns ErrAbort alongside a
// Result whose Query is still populated (the selection is empty).
type Result struct {
	IDs   []ID
	Items []string
	Query string
}

// state holds the picker's view of the feed. items, itemsLower, itemsStyled
// and ids are parallel, in feed order; everything else addresses an item by
// its position in them, except selection, which keys on ID.
type state struct {
	items       []string           // All item names (stripped of ansi when Opt.Ansi).
	itemsLower  []string           // Lowercased view of items for matching's hot path.
	itemsStyled [][]ansi.StyleSpan // Per-item style runs; non-nil only when Opt.Ansi.
	ids         []ID               // Feed handle of each item.
	pos         map[ID]int         // Inverse of ids.
	matched     []int              // Positions matching the input, ascending.

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
	// pageSize is the item-row count of the last frame, 0 before the first.
	pageSize int

	input []rune

	// selection holds the multi-select state. value = selection order (1-based).
	selection map[ID]int
	// selectionIdx holds the next index, which is used to a selection's value.
	selectionIdx int
}

// Screen is the terminal surface the picker draws to: the subset of
// tcell.Screen the finder actually uses. tcell.Screen, tcell.SimulationScreen,
// and litescreen.Screen all satisfy it implicitly.
//
// The zero-behaviour default (Opt.Screen == nil) is a litescreen backend the
// picker creates and owns. Inject your own via Opt.Screen to run on a
// different backend or headless; the picker then owns its lifecycle
// (Init / Fini / event wiring) for the duration of the run.
type Screen interface {
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
	term Screen
	// ownTerm reports whether the picker owns term's lifecycle (Init /
	// ChannelEvents / Fini). True when term came from Opt.Screen or was
	// created by initFinder; false when a test preset f.term directly, in
	// which case the test wires and tears down the screen itself.
	ownTerm   bool
	stateMu   sync.RWMutex
	state     state
	drawTimer *time.Timer
	eventCh   chan struct{}
	opt       *Opt
	multi     bool

	termEventsChan <-chan tcell.Event

	// PERF: cache strings.Fields(state.input) across draws. state.input
	// rarely changes between redraws but _draw runs many times per
	// keystroke, so recomputing fields per draw dominates the alloc
	// profile. drawWordsBuf holds a copy of the input that produced
	// drawWords; we invalidate by element-wise compare.
	drawWordsBuf []rune
	drawWords    []string

	// filterDone is a non-blocking signal sent by the bg event-loop
	// goroutine after a full filter+draw cycle. only used by tests to
	// sync on event processing without a fixed sleep. buffered 1 so
	// the goroutine never blocks when no test is waiting.
	filterDone chan struct{}

	// repaintPending asks the next frame to be a full repaint rather than a
	// diff. Set by the repaint ticker in aggressive mode, consumed (and
	// cleared) by flush. Atomic because it is written from the repaint
	// goroutine and read from the draw timer's.
	repaintPending atomic.Bool

	// termMu pins the screen's lifetime against the frames racing it, and
	// termDone records that it has been torn down. Every frame takes termMu
	// for its whole duration and teardown takes it to set termDone, so a
	// frame either completes before the screen goes away or is dropped.
	// Shutdown can't join the draw timer -- Stop doesn't wait for an AfterFunc
	// already running -- so this is what actually makes the ordering safe.
	termMu   sync.Mutex
	termDone bool

	// confirmedItems is the item text captured atomically with the confirmed
	// IDs (under the same lock), so a feed change landing between confirmation
	// and the caller reading the Result can't alter what is reported.
	confirmedItems []string
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

func (f *finder) initFinder(entries []entry, opt Opt) error {
	if f.term == nil {
		f.term = opt.Screen
	}
	if f.term == nil {
		screenH := opt.Height
		if screenH > 0 {
			screenH += chromeRows(opt)
		}
		s, err := litescreen.New(screenH)
		if err != nil {
			return fmt.Errorf("failed to new screen: %w", err)
		}
		f.term = s
	}
	// Screens from Opt.Screen and self-created ones are picker-owned; a
	// term preset before the call (tests) is caller-owned and arrives
	// already initialised with events wired.
	if f.termEventsChan == nil {
		f.ownTerm = true
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
		f.state.selection = map[ID]int{}
		f.state.selectionIdx = 1
	}

	items := make([]string, len(entries))
	f.state.ids = make([]ID, len(entries))
	f.state.pos = make(map[ID]int, len(entries))
	for i, e := range entries {
		items[i] = e.text
		f.state.ids[i] = e.id
		f.state.pos[e.id] = i
	}
	if opt.Ansi {
		stripped, styled := parseAnsiItems(items)
		f.state.items = stripped
		f.state.itemsStyled = styled
	} else {
		f.state.items = items
	}
	f.state.itemsLower = make([]string, len(f.state.items))
	for i, s := range f.state.items {
		f.state.itemsLower[i] = strings.ToLower(s)
	}
	f.state.matched = makeMatched(len(f.state.items))

	if !isInTesting() {
		// Create the draw timer stopped -- it only ever fires via draw()'s
		// Reset. A zero delay here would race the Stop below and could emit one
		// spurious frame before the first real draw, so start it far in the
		// future and cancel it immediately.
		f.drawTimer = time.AfterFunc(time.Hour, f.drawNow)
		f.drawTimer.Stop()
	}
	f.eventCh = make(chan struct{}, 30) // A large value
	f.filterDone = make(chan struct{}, 1)

	if opt.Query != "" {
		f.state.input = []rune(opt.Query)
		f.state.cursorX = runewidth.StringWidth(opt.Query)
		f.state.x = len(f.state.input)
		f.filter()
	}

	return nil
}

// applyDelta folds a feed delta into the picker's view. Only the changed items
// are parsed and matched, so a small change to a big list stays cheap. The
// cursor stays on its item; if that item is gone or no longer matches, it
// lands on the nearest surviving match.
func (f *finder) applyDelta(d delta) {
	if d.empty() {
		return
	}
	f.stateMu.Lock()
	defer f.stateMu.Unlock()

	cur := -1
	if f.state.y >= 0 && f.state.y < len(f.state.matched) {
		cur = f.state.matched[f.state.y]
	}
	if len(d.removed) > 0 {
		cur = f.removeLocked(d.removed, cur)
	}
	for _, e := range d.updated {
		f.updateLocked(e)
	}
	f.appendLocked(d.added)
	f.recursorLocked(cur)
}

// removeLocked drops ids from the view and compacts it. It returns where
// position cur ends up: unchanged for a survivor, the position of its
// successor if it was itself removed.
// Caller must hold f.stateMu.
func (f *finder) removeLocked(ids []ID, cur int) int {
	s := &f.state
	gone := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if p, ok := s.pos[id]; ok {
			gone[p] = struct{}{}
			delete(s.pos, id)
			delete(s.selection, id)
		}
	}
	if len(gone) == 0 {
		return cur
	}
	newPos := make([]int, len(s.items))
	newCur := cur
	w := 0
	for i := range s.items {
		if i == cur {
			newCur = w
		}
		if _, ok := gone[i]; ok {
			newPos[i] = -1
			continue
		}
		newPos[i] = w
		if w != i {
			s.items[w] = s.items[i]
			s.itemsLower[w] = s.itemsLower[i]
			if s.itemsStyled != nil {
				s.itemsStyled[w] = s.itemsStyled[i]
			}
			s.ids[w] = s.ids[i]
			s.pos[s.ids[w]] = w
		}
		w++
	}
	clear(s.items[w:])
	clear(s.itemsLower[w:])
	s.items, s.itemsLower, s.ids = s.items[:w], s.itemsLower[:w], s.ids[:w]
	if s.itemsStyled != nil {
		clear(s.itemsStyled[w:])
		s.itemsStyled = s.itemsStyled[:w]
	}
	matched := s.matched[:0]
	for _, m := range s.matched {
		if np := newPos[m]; np >= 0 {
			matched = append(matched, np)
		}
	}
	s.matched = matched
	return newCur
}

// updateLocked replaces one item's text in place and moves it into or out of
// matched as the query now dictates.
// Caller must hold f.stateMu.
func (f *finder) updateLocked(e entry) {
	s := &f.state
	p, ok := s.pos[e.id]
	if !ok {
		return
	}
	text, styled := f.parseItem(e.text)
	s.items[p] = text
	s.itemsLower[p] = strings.ToLower(text)
	if s.itemsStyled != nil {
		s.itemsStyled[p] = styled
	}
	match := len(f.matchLowerLocked(s.itemsLower[p:p+1])) == 1
	i, in := slices.BinarySearch(s.matched, p)
	switch {
	case match && !in:
		s.matched = slices.Insert(s.matched, i, p)
	case !match && in:
		s.matched = slices.Delete(s.matched, i, i+1)
	}
}

// appendLocked adds entries at the end of the view, matching only them.
// Caller must hold f.stateMu.
func (f *finder) appendLocked(entries []entry) {
	if len(entries) == 0 {
		return
	}
	s := &f.state
	start := len(s.items)
	for i, e := range entries {
		text, styled := f.parseItem(e.text)
		s.items = append(s.items, text)
		s.itemsLower = append(s.itemsLower, strings.ToLower(text))
		if s.itemsStyled != nil {
			s.itemsStyled = append(s.itemsStyled, styled)
		}
		s.ids = append(s.ids, e.id)
		s.pos[e.id] = start + i
	}
	for _, k := range f.matchLowerLocked(s.itemsLower[start:]) {
		s.matched = append(s.matched, start+k)
	}
}

// matchLowerLocked returns the indices into lower that match the current
// query. An empty query matches everything.
// Caller must hold f.stateMu.
func (f *finder) matchLowerLocked(lower []string) []int {
	if len(f.state.input) == 0 {
		return makeMatched(len(lower))
	}
	return matching.FindAllLower(strings.ToLower(string(f.state.input)), lower, f.negate())
}

// parseItem splits a raw feed item into its display text and, with Opt.Ansi,
// its style runs.
func (f *finder) parseItem(raw string) (string, []ansi.StyleSpan) {
	if f.opt == nil || !f.opt.Ansi {
		return raw, nil
	}
	return ansi.Strip(raw), ansi.ParseStyles(raw, tcell.StyleDefault)
}

// recursorLocked points the cursor at the first match at or after position
// cur, falling back to the last match. A cursor that didn't move keeps its
// screen row; one that did is re-aligned to the page.
// Caller must hold f.stateMu.
func (f *finder) recursorLocked(cur int) {
	s := &f.state
	n := len(s.matched)
	if n == 0 {
		s.y, s.cursorY = 0, 0
		return
	}
	y := 0
	if cur >= 0 {
		y, _ = slices.BinarySearch(s.matched, cur)
		y = min(y, n-1)
	}
	if y == s.y {
		return
	}
	s.y = y
	if s.pageSize > 0 {
		s.cursorY = y % s.pageSize
	} else {
		s.cursorY = min(s.cursorY, y)
	}
}

// cachedDrawWords returns strings.Fields(string(state.input)), reusing
// the previous result when state.input hasn't changed. _draw runs many
// times per keystroke; recomputing the fields each time was the
// dominant alloc in the finder render loop.
func (f *finder) cachedDrawWords() []string {
	in := f.state.input
	if len(in) == len(f.drawWordsBuf) {
		eq := true
		for i, r := range in {
			if r != f.drawWordsBuf[i] {
				eq = false
				break
			}
		}
		if eq {
			return f.drawWords
		}
	}
	f.drawWordsBuf = append(f.drawWordsBuf[:0], in...)
	f.drawWords = strings.Fields(string(in))
	return f.drawWords
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
	itemAreaHeight := max(height-firstItemOffset-1, 0)
	pageSize := itemAreaHeight + 1
	f.state.pageSize = pageSize
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
		maxPages = max((totalItems+pageSize-1)/pageSize, 1)
	}
	pw := len(fmt.Sprintf("%d", maxPages))

	yellow := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
	grey := tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorDefault)
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
	negMask := negRuneMask(f.state.input, f.negate())
	for i, r := range f.state.input {
		style := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault).Bold(true)
		if negMask != nil && negMask[i] {
			// Visual feedback that this term is negated. Red when colour is
			// allowed; strike-through when NO_COLOR, so the cue survives in
			// a monochrome terminal.
			if noColor() {
				style = style.StrikeThrough(true)
			} else {
				style = style.Foreground(tcell.ColorRed)
			}
		}
		f.term.SetContent(promptCol+w, promptRow, r, nil, style)
		w += runewidth.RuneWidth(r)
	}
	f.term.ShowCursor(promptCol+f.state.cursorX, promptRow)

	// Item lines: i=0 is the row closest to the prompt.
	matched := f.state.matched[topIdx:]
	words := f.cachedDrawWords()

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
			if _, ok := f.state.selection[f.state.ids[m]]; ok {
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
				for k := range wordRunes {
					if lowerItemRunes[i+k] != wordRunes[k] {
						match = false
						break
					}
				}
				if match {
					if highlightPositions == nil {
						highlightPositions = make(map[int]bool)
					}
					for k := range wordRunes {
						highlightPositions[i+k] = true
					}
				}
			}
		}

		var styled []ansi.StyleSpan
		if f.state.itemsStyled != nil && m < len(f.state.itemsStyled) {
			styled = f.state.itemsStyled[m]
		}
		plain := tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault)

		// Display-only items are drawn dimmed (a faint version of whatever
		// colour the runes already carry). The cursor row keeps its own
		// highlight -- being on the cursor is the stronger signal.
		unsel := f.unselectableLocked(m)

		w := 2
		for j, r := range itemRunes {
			style := ansi.StyleAt(styled, j, plain)
			hasHighlighted := highlightPositions[j]
			if hasHighlighted {
				style = tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorDefault)
			}
			if i == f.state.cursorY {
				if hasHighlighted {
					style = tcell.StyleDefault.Foreground(tcell.ColorTeal).Bold(true).Background(tcell.ColorBlack)
				} else {
					style = tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true).Background(tcell.ColorBlack)
				}
			} else if unsel {
				style = style.Dim(true)
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

		thumbSize := min(max(1, trackHeight*pageSize/total), trackHeight)
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

		trackStyle := tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorDefault)
		thumbStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorDefault)
		col := maxWidth - 1
		for row := range trackHeight {
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
	if isInTesting() {
		// Don't use goroutine scheduling.
		f.drawNow()
		return
	}
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	f.drawTimer.Reset(d)
}

// drawNow renders and emits one frame. termMu is held for the whole frame, so
// a teardown racing this either waits for the frame to finish or arrives
// first and the frame is dropped -- it can never land halfway, writing to a
// screen that has already been handed back.
func (f *finder) drawNow() {
	f.termMu.Lock()
	defer f.termMu.Unlock()
	if f.termDone {
		return
	}
	f.stateMu.Lock()
	f._draw()
	f.stateMu.Unlock()
	f.flush()
}

// stopDrawing disarms the draw timer so a frame a background goroutine has
// already queued never fires. Stop doesn't wait for an AfterFunc that has
// already begun; termDone covers that one.
func (f *finder) stopDrawing() {
	if f.drawTimer != nil {
		f.drawTimer.Stop()
	}
}

// finiTerm tears the screen down and blocks every later frame. Idempotent, and
// safe to call when the picker doesn't own the screen -- the caller's screen is
// left alone, but draws still stop.
func (f *finder) finiTerm() {
	f.termMu.Lock()
	defer f.termMu.Unlock()
	if f.termDone {
		return
	}
	f.termDone = true
	if f.ownTerm {
		f.term.Fini()
	}
}

// flush emits the current frame: a full repaint when one has been requested
// since the last frame, otherwise the cheaper diff-based Show. Consuming the
// request here (rather than keying off RedrawAggressive directly) keeps the
// expensive path on the repaint ticker's cadence instead of every draw.
func (f *finder) flush() {
	if f.repaintPending.Swap(false) {
		f.term.Sync()
		return
	}
	f.term.Show()
}

// requestRepaint marks the next frame as a full repaint and wakes the draw
// loop. Non-blocking: a full eventCh means a redraw is already queued, which
// is all we need -- repaintPending is sticky until some frame consumes it.
func (f *finder) requestRepaint() {
	f.repaintPending.Store(true)
	select {
	case f.eventCh <- struct{}{}:
	default:
	}
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
	pageSize := max(screenHeight-firstItemOffset, 1)

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
			// Kill the word before the cursor: skip back over trailing
			// whitespace, then over the word itself. Everything from the
			// cursor on is spliced back on -- ctrl+w is a delete-backwards,
			// not a truncate, so text to the right of the cursor survives.
			x := f.state.x
			start := x
			for start > 0 && unicode.IsSpace(f.state.input[start-1]) {
				start--
			}
			for start > 0 && !unicode.IsSpace(f.state.input[start-1]) {
				start--
			}
			if start == x {
				return nil // nothing before the cursor to kill
			}
			f.state.input = append(f.state.input[:start], f.state.input[x:]...)
			f.state.cursorX = runewidth.StringWidth(string(f.state.input[:start]))
			f.state.x = start
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
			if len(f.state.matched) == 0 {
				return nil
			}
			idx := f.state.matched[f.state.y]
			// Unselectable items can't be toggled, but Tab still advances the
			// cursor like a normal item.
			if !f.unselectableLocked(idx) {
				id := f.state.ids[idx]
				if _, ok := f.state.selection[id]; ok {
					delete(f.state.selection, id)
				} else {
					f.state.selection[id] = f.state.selectionIdx
					f.state.selectionIdx++
				}
			}
			f.scrollAwayFromPrompt(pageSize, matchedLinesCount)
		default:
			// Only a plain rune types. Ctrl-letter events carry the letter as
			// their rune and Alt-x carries x; both are unbound, not text.
			if e.Key() == tcell.KeyRune && e.Modifiers()&^tcell.ModShift == 0 && e.Rune() != 0 {
				width, _ := f.term.Size()
				maxLineWidth := width - 2 - 1
				// Measure display columns, not rune count -- wide (CJK) runes
				// take two columns each, so a rune-count cap lets them overflow.
				if runewidth.StringWidth(string(f.state.input))+runewidth.RuneWidth(e.Rune()) > maxLineWidth {
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
		// Recompute cursorY for the new page size (strict alignment). Use the
		// same chrome accounting as the normal path and _draw -- the number-line
		// shares the prompt row, so chrome above items is chromeRows(opt).
		newPageSize := height - chromeRows(*f.opt)
		if newPageSize > 0 {
			f.state.cursorY = f.state.y % newPageSize
		}

		maxLineWidth := width - 2 - 1
		if maxLineWidth < 0 {
			f.state.input = nil
			f.state.cursorX = 0
			f.state.x = 0
		} else if runewidth.StringWidth(string(f.state.input)) > maxLineWidth {
			// Shrunk below the query width: truncate by display columns so wide
			// runes don't push the visible line past the new edge.
			trunc := []rune(runewidth.Truncate(string(f.state.input), maxLineWidth, ""))
			f.state.input = trunc
			f.state.cursorX = runewidth.StringWidth(string(trunc))
			f.state.x = len(trunc)
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
	matchedItems := matching.FindAllLower(strings.ToLower(string(f.state.input)), f.state.itemsLower, f.negate())
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

const (
	// frameInterval is the shortest gap between two feed-driven frames. A
	// fast producer wakes the picker per item; changes arriving inside one
	// interval are folded into a single frame.
	frameInterval = 16 * time.Millisecond
	// repaintInterval is how often Opt.RedrawAggressive forces a full repaint
	// to paint over a sibling process's writes. Deliberately far slower than
	// frameInterval: a repaint is the whole grid re-emitted, and doing that at
	// frame rate keeps the cursor hidden for a visible slice of every frame.
	// This is the ceiling on how long tearing can stay on screen.
	repaintInterval = 250 * time.Millisecond
)

// find runs the picker against a Feed. The picker takes the feed's current
// contents, then folds in each change as the feed reports it.
func (f *finder) find(ctx context.Context, feed *Feed, opt Opt) ([]ID, error) {
	if feed == nil {
		return nil, errors.New("feed must not be nil")
	}

	opt = opt.withDefaults()
	f.multi = opt.Multi

	if opt.SelectOne {
		// Only a finished stream can show there's exactly one match, so
		// decide after Close -- and before taking over the terminal, so a
		// sole match never flashes the UI.
		select {
		case <-feed.Done():
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if e, ok := soleMatch(feed.entries(), opt); ok {
			f.confirmedItems = []string{e.text}
			return []ID{e.id}, nil
		}
	}

	// Init before spawning any goroutine: on failure we return here and
	// nothing is left running.
	if err := f.initFinder(feed.attach(), opt); err != nil {
		return nil, fmt.Errorf("failed to initialize the fuzzy finder: %w", err)
	}

	// Teardown runs in the reverse of this registration order, and the order
	// matters: cancel first so the background goroutines stop queueing work,
	// then drop any frame already queued, and only then hand the terminal
	// back. Getting this backwards -- which is what a defer inside runLoop
	// did, since that returns before find unwinds -- let a queued redraw
	// write onto a terminal already restored to cooked mode.
	defer f.finiTerm()
	defer f.stopDrawing()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opt.RedrawAggressive {
		// The repaint runs for as long as the picker is open, not just while
		// items are arriving: the producer's stdout (our items) and stderr
		// (the bytes that tear us) are independent streams, so a feed that
		// has gone quiet is no evidence the producer has stopped writing to
		// the terminal. `find / -name '*.foo'` spends most of its life in
		// exactly that state.
		go func() {
			ticker := time.NewTicker(repaintInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					f.requestRepaint()
				}
			}
		}()
	}

	return f.runLoop(ctx, feed)
}

// soleMatch reports the one item matching opt.Query, if there is exactly one
// and it is selectable. Its text comes back ANSI-stripped when Opt.Ansi.
func soleMatch(entries []entry, opt Opt) (entry, bool) {
	lower := make([]string, len(entries))
	for i := range entries {
		if opt.Ansi {
			entries[i].text = ansi.Strip(entries[i].text)
		}
		lower[i] = strings.ToLower(entries[i].text)
	}
	hits := matching.FindAllLower(strings.ToLower(opt.Query), lower, opt.Negate)
	if len(hits) != 1 {
		return entry{}, false
	}
	e := entries[hits[0]]
	if opt.Unselectable != nil && opt.Unselectable(e.text) {
		return entry{}, false
	}
	return e, true
}

func makeMatched(n int) []int {
	matched := make([]int, n)
	for i := range matched {
		matched[i] = i
	}
	return matched
}

// parseAnsiItems converts raw ANSI-coloured input items into stripped
// match-friendly strings + parallel style-run slices for drawing. Both
// outputs are parallel-indexed with the input.
func parseAnsiItems(items []string) ([]string, [][]ansi.StyleSpan) {
	stripped := make([]string, len(items))
	styled := make([][]ansi.StyleSpan, len(items))
	for i, raw := range items {
		styled[i] = ansi.ParseStyles(raw, tcell.StyleDefault)
		stripped[i] = ansi.Strip(raw)
	}
	return stripped, styled
}

// runLoop drives the picker until the user confirms, aborts, or ctx ends.
// Teardown belongs to find, not here: a defer in this function runs while
// find's own goroutines are still live, which is the ordering bug it used to
// have.
func (f *finder) runLoop(ctx context.Context, feed *Feed) ([]ID, error) {
	// This goroutine is the only writer of the item view: feed deltas and
	// query re-filters are serialised here, so a filter can never install a
	// match computed against items a delta has since replaced.
	go func() {
		wake := feed.notify
		var throttle <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-f.eventCh:
				f.filter()
			case <-wake:
				d := feed.drain()
				if d.empty() {
					continue
				}
				f.applyDelta(d)
				// Hold further wakeups for a frame; they coalesce in the
				// feed and the next drain takes them all at once.
				wake, throttle = nil, time.After(frameInterval)
			case <-throttle:
				wake, throttle = feed.notify, nil
				continue
			}
			f.draw(0)
			// non-blocking signal so tests can sync on a fully
			// processed event (filter + draw). buffered 1 +
			// non-blocking send means production never stalls
			// here when nothing is waiting.
			select {
			case f.filterDone <- struct{}{}:
			default:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			f.draw(10 * time.Millisecond)

			f.stateMu.RLock()
			prevInputLen := len(f.state.input)
			f.stateMu.RUnlock()

			err := f.readKey(ctx)

			// in tests, sync on filter completion so subsequent reads
			// of state.matched (snapshot tests) and the next iteration
			// observe a consistent state. only wait when input changed,
			// matching the eventCh push in readKey's defer; otherwise
			// the bg goroutine wouldn't be running filter and we'd
			// block on a signal that won't come.
			if isInTesting() {
				f.stateMu.RLock()
				curInputLen := len(f.state.input)
				f.stateMu.RUnlock()
				if prevInputLen != curInputLen {
					select {
					case <-f.filterDone:
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
			switch {
			case errors.Is(err, ErrAbort):
				return nil, ErrAbort
			case errors.Is(err, errEntered):
				idxs, done := f.confirmSelection()
				if !done {
					// Cursored item is unselectable; ignore Enter, stay open.
					continue
				}
				return idxs, nil
			case err != nil:
				return nil, fmt.Errorf("failed to read a key: %w", err)
			}
		}
	}
}

// Find displays a fuzzy-finder UI over a Feed and returns the selection, or
// ErrAbort if the user cancels. With Opt.Multi=false the result carries at
// most one element. See Result for the empty-selection case.
//
// The feed may change while the picker is open: producers Add, Update and
// Remove on it from any goroutine and the picker shows each change within a
// frame. For a static list, use NewFeedFrom.
func Find(ctx context.Context, feed *Feed, opt Opt) (Result, error) {
	f := &finder{}
	return f.Find(ctx, feed, opt)
}

// Find is the picker-method form, used by tests that need to inject
// a mocked terminal.
func (f *finder) Find(ctx context.Context, feed *Feed, opt Opt) (Result, error) {
	return f.result(f.find(ctx, feed, opt))
}

// result packages a finished run's IDs into a Result, attaching the item text
// captured with them and the query as typed on exit. Errors pass through, but
// the query still rides along: a cancelled picker can tell the caller what was
// typed.
func (f *finder) result(ids []ID, err error) (Result, error) {
	res := Result{Query: f.query()}
	if err != nil {
		return res, err
	}
	res.IDs = ids
	res.Items = f.confirmedItems
	return res, nil
}

// query returns the picker's query as typed. Safe before initFinder has run
// (returns ""), which is the case when find() bails on a nil feed.
func (f *finder) query() string {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	return string(f.state.input)
}

// negate reports whether fzf-style '!' negative needles are enabled.
func (f *finder) negate() bool {
	return f.opt != nil && f.opt.Negate
}

// noColor reports whether NO_COLOR is set (any value), per no-color.org.
func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// negRuneMask marks which runes of input belong to a negated ('!'-prefixed)
// term, for query-line highlighting. Returns nil when negate is off or no term
// is negated (no allocation in the common case).
//
// Word splitting and the negated-term test mirror matching.parse: terms are
// split on any Unicode whitespace (as strings.Fields does), and a bare "!" is
// not a negated term (parse drops it), so it isn't highlighted.
func negRuneMask(input []rune, negate bool) []bool {
	if !negate {
		return nil
	}
	var mask []bool
	for i := 0; i < len(input); {
		if unicode.IsSpace(input[i]) {
			i++
			continue
		}
		start := i
		for i < len(input) && !unicode.IsSpace(input[i]) {
			i++
		}
		// Negated iff the term starts with '!' and has more than just '!'.
		if input[start] == '!' && i-start > 1 {
			if mask == nil {
				mask = make([]bool, len(input))
			}
			for j := start; j < i; j++ {
				mask[j] = true
			}
		}
	}
	return mask
}

// unselectableLocked reports whether the item at idx is display-only per
// Opt.Unselectable. Caller must hold f.stateMu (read or write).
func (f *finder) unselectableLocked(idx int) bool {
	if f.opt == nil || f.opt.Unselectable == nil {
		return false
	}
	if idx < 0 || idx >= len(f.state.items) {
		return false
	}
	return f.opt.Unselectable(f.state.items[idx])
}

// anySelectableLocked reports whether any currently matched item can be
// confirmed. False when nothing matches, or when Opt.Unselectable rules out
// every match. Caller must hold f.stateMu (read or write).
func (f *finder) anySelectableLocked() bool {
	for _, idx := range f.state.matched {
		if !f.unselectableLocked(idx) {
			return true
		}
	}
	return false
}

// confirmSelection resolves what Enter should do. Three outcomes:
//   - (idxs, true): confirmed selection
//   - ([]int{}, true): no matched item is selectable, so the picker cannot
//     produce a selection -- end it with an empty result rather than trapping
//     the user (Enter would otherwise be a dead key).
//   - (nil, false): selectable matches exist but the cursor sits on an
//     unselectable one; swallow Enter and keep the picker open.
//
// The distinction between the last two is the whole point: a dimmed row under
// the cursor is a "not this one" nudge, whereas a wholly-unselectable match set
// is a picker with nothing to pick.
func (f *finder) confirmSelection() ([]ID, bool) {
	// Write lock, not read: the confirmedItems snapshot below is a write to
	// f, taken here so it lands atomically with the indices it describes.
	f.stateMu.Lock()
	defer f.stateMu.Unlock()

	// Selection is built via Tab, which already rejects unselectable items, so
	// every entry here is selectable -- and it survives the match set being
	// filtered down to nothing, so it's checked before anySelectableLocked.
	if f.multi && len(f.state.selection) > 0 {
		picks := make([]selectedItem, 0, len(f.state.selection))
		for id, pos := range f.state.selection {
			picks = append(picks, selectedItem{id: id, order: pos})
		}
		slices.SortFunc(picks, func(a, b selectedItem) int { return a.order - b.order })
		ids := make([]ID, len(picks))
		for i, p := range picks {
			ids[i] = p.id
		}
		f.confirmedItems = f.itemsForLocked(ids)
		return ids, true
	}
	if !f.anySelectableLocked() {
		f.confirmedItems = []string{}
		return []ID{}, true
	}
	cur := f.state.matched[f.state.y]
	if f.unselectableLocked(cur) {
		return nil, false
	}
	id := f.state.ids[cur]
	f.confirmedItems = f.itemsForLocked([]ID{id})
	return []ID{id}, true
}

// selectedItem pairs an ID with its 1-based multi-select order so the two
// survive a sort together. Sorting an ID slice against a parallel order slice
// does not work: the sort permutes only the first, so the comparator ends up
// reading orders belonging to other IDs.
type selectedItem struct {
	id    ID
	order int
}

// itemsForLocked translates IDs into their current item text. IDs no longer in
// the view are dropped. Caller must hold f.stateMu (read or write).
func (f *finder) itemsForLocked(ids []ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if p, ok := f.state.pos[id]; ok {
			out = append(out, f.state.items[p])
		}
	}
	return out
}

func isInTesting() bool {
	return flag.Lookup("test.v") != nil
}
