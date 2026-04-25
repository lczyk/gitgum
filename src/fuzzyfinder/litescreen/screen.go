// Package litescreen is a minimal tcell-free terminal renderer. It opens
// /dev/tty directly so the process's stdin/stdout remain free for piping,
// and implements a subset of the tcell.Screen surface (Init/Fini/Size/
// SetContent/Clear/Show/ShowCursor/ChannelEvents) sufficient for fuzzy-
// finder-style TUIs. The intended downstream is gitgum/fuzzyfinder, but the
// package has no internal dependencies and can be used standalone.
//
// The renderer was developed based on fzf's LightRenderer (junegunn/fzf,
// src/tui/light.go). The architecture, ANSI handling approach, and several
// specific techniques — opening /dev/tty for I/O, using DSR (\e[6n) to
// discover cursor position, scrolling via makeSpace, the ESC-disambiguation
// timeout — all come from there. Code is freshly written, not copied, but
// credit is due. fzf is MIT-licensed; see LICENSE-fzf at the repo root.
package litescreen

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// Screen is a tcell-free terminal renderer. It keeps a back buffer of cells;
// Show diffs against a front buffer and emits only the deltas as ANSI
// sequences. Input is parsed off /dev/tty in a goroutine, with a short
// timeout to disambiguate bare ESC from the start of an escape sequence.
//
// Two modes selected by the height parameter to New:
//   - fullscreen (height == 0 or rows == termH): alt-screen entered with
//     \e[?1049h, restored on Fini
//   - inline (height nonzero, less than termH): occupies last N rows of
//     the main screen, preserving prior terminal output above
//
// The current mode is encoded in yOrigin: yOrigin == 0 means fullscreen,
// yOrigin > 0 means inline.
type Screen struct {
	in  *os.File
	out *os.File

	rawState *term.State

	height  int // raw height arg to New; see resolveHeight
	yOrigin int // first row of region (0-indexed); 0 in fullscreen

	mu               sync.Mutex
	fb               *framebuf
	cursorX, cursorY int
	cursorVisible    bool

	winch chan os.Signal

	// signal-driven cleanup. SIGINT/SIGTERM mid-picker leaves terminal in
	// raw mode + alt-screen + cursor hidden if we don't intercept. cleanup
	// runs at most once, from either Fini or the signal goroutine.
	sigCh       chan os.Signal
	cleanupOnce sync.Once
}

// resolveHeight turns the user-supplied Height value into an actual row
// count given the current terminal height. Returns (rows, fullscreen).
//
//	h == 0           fullscreen
//	h > 0            exactly h rows
//	h < 0            termH + h (e.g. -2 → leave 2 rows visible above)
//
// Clamps to [1, termH]; if the request meets or exceeds termH, returns
// fullscreen.
func resolveHeight(h, termH int) (int, bool) {
	if h == 0 {
		return termH, true
	}
	rows := h
	if rows < 0 {
		rows = termH + rows
	}
	if rows < 1 {
		rows = 1
	}
	if rows >= termH {
		return termH, true
	}
	return rows, false
}

type liteCell struct {
	mainc rune
	combc []rune
	style tcell.Style
}

// framebuf is the rendering compositor: holds the back (intended) and front
// (last-drawn) cell grids and produces ANSI byte streams that transform
// front into back. Pulled out of Screen so the diff/SGR/wide-rune logic
// is testable without /dev/tty.
type framebuf struct {
	width, height int
	back, front   [][]liteCell
}

func newFramebuf(w, h int) *framebuf {
	f := &framebuf{}
	f.resize(w, h)
	return f
}

// resize (re)allocates the buffers to the given dimensions. front is filled
// with a sentinel (mainc=-1) so the next flush emits every cell;
// back is filled with blanks.
func (f *framebuf) resize(w, h int) {
	f.width, f.height = w, h
	f.front = make([][]liteCell, h)
	f.back = make([][]liteCell, h)
	for y := 0; y < h; y++ {
		f.front[y] = make([]liteCell, w)
		f.back[y] = make([]liteCell, w)
		for x := 0; x < w; x++ {
			f.front[y][x] = liteCell{mainc: -1}
			f.back[y][x] = liteCell{mainc: ' '}
		}
	}
}

// clear blanks the back buffer.
func (f *framebuf) clear() {
	for y := 0; y < f.height; y++ {
		for x := 0; x < f.width; x++ {
			f.back[y][x] = liteCell{mainc: ' '}
		}
	}
}

// set writes a cell into the back buffer, ignoring out-of-range coordinates.
func (f *framebuf) set(x, y int, c liteCell) {
	if x < 0 || x >= f.width || y < 0 || y >= f.height {
		return
	}
	f.back[y][x] = c
}

// flush returns the byte stream that updates the terminal from front to
// back, optionally repositioning the cursor afterwards. yOrigin is the row
// (0-indexed in the absolute terminal) where row 0 of the framebuf lives.
// Pass 0 for fullscreen mode; pass termH-rows for inline mode.
//
// As a side effect, front is updated to match back so a subsequent flush
// with no intervening changes emits an empty payload (modulo the
// cursor-hide/show framing).
func (f *framebuf) flush(yOrigin, cx, cy int, cursorVisible bool) []byte {
	var buf bytes.Buffer

	// Hide cursor + disable auto-wrap (DECAWM) while drawing. Without ?7l,
	// emitting a cell in the bottom-right corner advances the cursor to a
	// new line — at the bottom of the terminal that triggers a scroll,
	// which moves our region under us and corrupts subsequent renders.
	// Restored at end of flush.
	buf.WriteString("\x1b[?25l\x1b[?7l")

	var prevStyle tcell.Style
	var styleSet bool

	for y := 0; y < f.height; y++ {
		for x := 0; x < f.width; x++ {
			c := f.back[y][x]
			if cellEqual(c, f.front[y][x]) {
				// Wide cells leave a phantom slot; if main cell unchanged,
				// the phantom doesn't need re-emission either.
				w := runewidth.RuneWidth(c.mainc)
				if w > 1 {
					x += w - 1
				}
				continue
			}

			// Cursor positioning is 1-indexed and absolute. Add yOrigin to
			// shift fb row 0 to the region's first terminal row.
			fmt.Fprintf(&buf, "\x1b[%d;%dH", yOrigin+y+1, x+1)

			if !styleSet || c.style != prevStyle {
				buf.WriteString("\x1b[m")
				buf.WriteString(styleToSGR(c.style))
				prevStyle = c.style
				styleSet = true
			}

			r := c.mainc
			if r == 0 {
				r = ' '
			}
			buf.WriteRune(r)
			for _, cr := range c.combc {
				buf.WriteRune(cr)
			}

			f.front[y][x] = c

			w := runewidth.RuneWidth(c.mainc)
			if w > 1 {
				// Wide cell: mark phantom slot consumed.
				if x+1 < f.width {
					f.front[y][x+1] = c
				}
				x += w - 1
			}
		}
	}

	buf.WriteString("\x1b[m\x1b[?7h") // restore auto-wrap
	if cursorVisible {
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[?25h", yOrigin+cy+1, cx+1)
	}
	return buf.Bytes()
}

// New constructs a Screen with the given height policy:
//
//	0   fullscreen (alt-screen, preserves nothing above)
//	N>0 exactly N rows at the bottom; prior output preserved above
//	N<0 terminal_rows + N (e.g. -2 leaves 2 rows visible above)
func New(height int) (*Screen, error) {
	in, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/tty for read: %w", err)
	}
	out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		in.Close()
		return nil, fmt.Errorf("open /dev/tty for write: %w", err)
	}
	return &Screen{in: in, out: out, height: height}, nil
}

func (s *Screen) Init() error {
	st, err := term.MakeRaw(int(s.in.Fd()))
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	s.rawState = st

	w, termH := s.terminalSize()
	rows, fullscreen := resolveHeight(s.height, termH)
	s.fb = newFramebuf(w, rows)

	if fullscreen {
		// Enter alt-screen, clear, home cursor, hide cursor.
		io.WriteString(s.out, "\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l")
		s.yOrigin = 0
	} else {
		// Inline mode: scroll content up by emitting (rows-1) newlines so
		// we have N free lines at the bottom of the terminal. After the
		// LFs the cursor is clamped to the last row, so the region's first
		// row is termH-rows.
		io.WriteString(s.out, "\x1b[?25l")
		for i := 0; i < rows-1; i++ {
			io.WriteString(s.out, "\n")
		}
		s.yOrigin = termH - rows
		// Clear the region in case prior content overlaps.
		fmt.Fprintf(s.out, "\x1b[%d;1H\x1b[J", s.yOrigin+1)
	}
	s.cursorVisible = false

	s.winch = make(chan os.Signal, 1)
	signal.Notify(s.winch, syscall.SIGWINCH)

	s.sigCh = make(chan os.Signal, 1)
	signal.Notify(s.sigCh, syscall.SIGINT, syscall.SIGTERM)
	go s.signalLoop()
	return nil
}

// signalLoop catches SIGINT/SIGTERM, restores the terminal, and re-raises
// the signal so the default handler runs (process exit with the correct
// status). Without this, a Ctrl-C delivered while the picker is up leaves
// the terminal in raw mode + cursor hidden.
func (s *Screen) signalLoop() {
	sig, ok := <-s.sigCh
	if !ok {
		return
	}
	s.cleanup()
	signal.Reset(sig.(syscall.Signal))
	syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))
}

// cleanup restores the terminal to its pre-Init state. Idempotent: safe to
// call from both Fini and the signal goroutine. Mode is derived from
// yOrigin: 0 means fullscreen (alt-screen needs rmcup), nonzero means
// inline (clear region).
func (s *Screen) cleanup() {
	s.cleanupOnce.Do(func() {
		if s.yOrigin == 0 {
			io.WriteString(s.out, "\x1b[m\x1b[?25h\x1b[?1049l")
		} else {
			fmt.Fprintf(s.out, "\x1b[m\x1b[%d;1H\x1b[J\x1b[?25h", s.yOrigin+1)
		}
		if s.rawState != nil {
			term.Restore(int(s.in.Fd()), s.rawState)
			s.rawState = nil
		}
	})
}

func (s *Screen) Fini() {
	s.cleanup()

	if s.winch != nil {
		signal.Stop(s.winch)
		close(s.winch)
		s.winch = nil
	}
	if s.sigCh != nil {
		signal.Stop(s.sigCh)
		close(s.sigCh)
		s.sigCh = nil
	}
	s.in.Close()
	s.out.Close()
}

func (s *Screen) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fb.width, s.fb.height
}

func (s *Screen) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fb.clear()
}

func (s *Screen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fb.set(x, y, liteCell{mainc: mainc, combc: combc, style: style})
}

func (s *Screen) ShowCursor(x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursorX, s.cursorY = x, y
	s.cursorVisible = true
}

func (s *Screen) Show() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out.Write(s.fb.flush(s.yOrigin, s.cursorX, s.cursorY, s.cursorVisible))
}

func cellEqual(a, b liteCell) bool {
	if a.mainc != b.mainc || a.style != b.style || len(a.combc) != len(b.combc) {
		return false
	}
	for i := range a.combc {
		if a.combc[i] != b.combc[i] {
			return false
		}
	}
	return true
}

// terminalSize queries the terminal connected to s.out for its size, falling
// back to 80x24 on error so we always have a usable framebuf.
func (s *Screen) terminalSize() (int, int) {
	w, h, err := term.GetSize(int(s.out.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// styleToSGR converts a tcell.Style to the corresponding ANSI SGR sequence.
// Mirrors mock.go's parseAttr; keeping the two in sync is intentional.
func styleToSGR(st tcell.Style) string {
	fg, bg, attr := st.Decompose()
	var params []string
	if attr&tcell.AttrBold != 0 {
		params = append(params, "1")
	}
	if attr&tcell.AttrDim != 0 {
		params = append(params, "2")
	}
	if attr&tcell.AttrItalic != 0 {
		params = append(params, "3")
	}
	if attr&tcell.AttrUnderline != 0 {
		params = append(params, "4")
	}
	if attr&tcell.AttrBlink != 0 {
		params = append(params, "5")
	}
	if attr&tcell.AttrReverse != 0 {
		params = append(params, "7")
	}
	if attr&tcell.AttrStrikeThrough != 0 {
		params = append(params, "9")
	}
	switch {
	case fg == tcell.ColorDefault:
	case fg > tcell.Color255:
		r, g, b := fg.RGB()
		params = append(params, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("38;5;%d", fg-tcell.ColorValid))
	}
	switch {
	case bg == tcell.ColorDefault:
	case bg > tcell.Color255:
		r, g, b := bg.RGB()
		params = append(params, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	default:
		params = append(params, fmt.Sprintf("48;5;%d", bg-tcell.ColorValid))
	}
	if len(params) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(params, ";") + "m"
}

// ChannelEvents reads from /dev/tty in raw mode, parses keystrokes and
// SIGWINCH, and emits tcell events on out. Stops when quit closes or input
// EOFs. The parser handles UTF-8 multibyte runes, control bytes, and the
// CSI/SS3 escape sequences the finder cares about.
func (s *Screen) ChannelEvents(out chan<- tcell.Event, quit <-chan struct{}) {
	// Don't close `out` on exit: the finder's receive loop doesn't check
	// `ok` and would spin on the closed channel.
	bytesCh := make(chan byte, 256)
	readErr := make(chan error, 1)
	go s.readLoop(bytesCh, readErr)

	for {
		select {
		case <-quit:
			return
		case <-s.winch:
			w, h := s.handleResize()
			select {
			case out <- tcell.NewEventResize(w, h):
			case <-quit:
				return
			}
		case <-readErr:
			return
		case b, ok := <-bytesCh:
			if !ok {
				return
			}
			ev := s.parseEvent(b, bytesCh)
			if ev != nil {
				select {
				case out <- ev:
				case <-quit:
					return
				}
			}
		}
	}
}

func (s *Screen) readLoop(out chan<- byte, errCh chan<- error) {
	buf := make([]byte, 64)
	for {
		n, err := s.in.Read(buf)
		if err != nil {
			errCh <- err
			close(out)
			return
		}
		for i := 0; i < n; i++ {
			out <- buf[i]
		}
	}
}

func (s *Screen) handleResize() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, termH := s.terminalSize()
	rows, fullscreen := resolveHeight(s.height, termH)

	prevWidth := s.fb.width
	if fullscreen {
		s.yOrigin = 0
		io.WriteString(s.out, "\x1b[2J\x1b[H")
	} else {
		s.yOrigin = termH - rows
		if w < prevWidth {
			// Narrowing reflows previous picker rows into more visual lines
			// that scroll up past yOrigin, leaving wrapped tails above it.
			// We can't clear them precisely without knowing how much wrap
			// happened, so wipe the entire visible viewport. Scrollback is
			// untouched — pre-picker output is still scrollable up.
			// Its not the best thing to do here, but i cant think of anythig
			// better atm..
			io.WriteString(s.out, "\x1b[2J")
		}
		fmt.Fprintf(s.out, "\x1b[%d;1H\x1b[J", s.yOrigin+1)
	}
	s.fb.resize(w, rows)
	return w, rows
}

const escTimeout = 50 * time.Millisecond

// parseEvent consumes b plus any follow-up bytes needed to form a complete
// event, and returns it. May read additional bytes from ch (for escape
// sequences and UTF-8 continuation bytes). Returns nil if the bytes don't
// form a recognized event.
func (s *Screen) parseEvent(b byte, ch <-chan byte) tcell.Event {
	switch {
	case b == 0x1b:
		return parseEscape(ch)
	case b == 0x7f:
		return tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	case b < 0x20:
		return controlByteEvent(b)
	case b < 0x80:
		// Plain ASCII printable.
		return tcell.NewEventKey(tcell.KeyRune, rune(b), tcell.ModNone)
	default:
		// UTF-8 multibyte sequence.
		return parseUTF8(b, ch)
	}
}

// controlByteEvent maps an ASCII control byte (0x00-0x1F) to a tcell event.
// Naming gotcha: tcell's KeyCtrlA..KeyCtrlZ live at values 65..90, NOT at
// the byte values 1..26 — those byte ranges are KeyNUL..KeySUB (KeyETX, etc).
// The finder switches on KeyCtrlC, so byte 0x03 must produce that constant,
// not Key(3). The "directly typeable" specials (Backspace, Tab, Enter, Esc)
// retain their byte-valued aliases since the finder switches on those names.
func controlByteEvent(b byte) tcell.Event {
	switch b {
	case 0x00:
		return tcell.NewEventKey(tcell.KeyCtrlSpace, ' ', tcell.ModCtrl)
	case 0x08:
		return tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone)
	case 0x09:
		return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	case 0x0d:
		return tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	case 0x1b:
		return tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
	}
	if b >= 0x01 && b <= 0x1A {
		// Map to KeyCtrlA..KeyCtrlZ, with rune set to the lowercase letter
		// the user pressed (matches tcell's input layer).
		return tcell.NewEventKey(tcell.KeyCtrlA+tcell.Key(b-1), rune(b)+'`', tcell.ModCtrl)
	}
	// Other rare control bytes (0x1c-0x1f). Pass the raw value through;
	// not used by the finder.
	return tcell.NewEventKey(tcell.Key(b), 0, tcell.ModCtrl)
}

func parseEscape(ch <-chan byte) tcell.Event {
	// After ESC, wait briefly for follow-up bytes. If none, it's a bare ESC.
	timer := time.NewTimer(escTimeout)
	defer timer.Stop()

	var b byte
	select {
	case <-timer.C:
		return tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
	case bb, ok := <-ch:
		if !ok {
			return tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
		}
		b = bb
	}

	switch b {
	case '[':
		return parseCSI(ch)
	case 'O':
		return parseSS3(ch)
	}
	// ESC + char with no recognised intro — treat as bare ESC and drop char.
	// (Could also emit Alt+char if we cared.)
	return tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone)
}

// parseCSI reads bytes after "ESC [" until a final byte (0x40-0x7e) and
// dispatches to a tcell key. Recognised: arrows, Home, End, PgUp, PgDn,
// Insert, Delete.
func parseCSI(ch <-chan byte) tcell.Event {
	var params []byte
	timer := time.NewTimer(escTimeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return nil
		case b, ok := <-ch:
			if !ok {
				return nil
			}
			if b >= 0x40 && b <= 0x7e {
				return mapCSI(string(params), b)
			}
			params = append(params, b)
			// Reset the timer for the next byte; CSI sequences arrive
			// contiguously so any gap means the sequence is malformed.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(escTimeout)
		}
	}
}

func mapCSI(params string, final byte) tcell.Event {
	switch final {
	case 'A':
		return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	case 'B':
		return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	case 'C':
		return tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
	case 'D':
		return tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
	case 'H':
		return tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)
	case 'F':
		return tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)
	case '~':
		switch params {
		case "1", "7":
			return tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)
		case "2":
			return tcell.NewEventKey(tcell.KeyInsert, 0, tcell.ModNone)
		case "3":
			return tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone)
		case "4", "8":
			return tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)
		case "5":
			return tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)
		case "6":
			return tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone)
		}
	}
	return nil
}

// parseSS3 handles "ESC O X" sequences. tmux, emacs, and some terminal
// configs emit SS3-form arrow keys (\x1bOA etc.) instead of the more common
// CSI form (\x1b[A). Both must work, so we map the same final bytes here as
// in parseCSI.
func parseSS3(ch <-chan byte) tcell.Event {
	timer := time.NewTimer(escTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case b, ok := <-ch:
		if !ok {
			return nil
		}
		switch b {
		case 'A':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case 'B':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'C':
			return tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
		case 'D':
			return tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
		case 'H':
			return tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)
		case 'F':
			return tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)
		}
	}
	return nil
}

// parseUTF8 collects continuation bytes for a multibyte rune.
func parseUTF8(lead byte, ch <-chan byte) tcell.Event {
	want := utf8ContinuationCount(lead)
	if want == 0 {
		return nil
	}
	buf := []byte{lead}
	timer := time.NewTimer(escTimeout)
	defer timer.Stop()

	for i := 0; i < want; i++ {
		select {
		case <-timer.C:
			return nil
		case b, ok := <-ch:
			if !ok {
				return nil
			}
			buf = append(buf, b)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(escTimeout)
	}
	r, _ := utf8.DecodeRune(buf)
	if r == utf8.RuneError {
		return nil
	}
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
}

// utf8ContinuationCount returns how many continuation bytes follow the lead
// byte b. 0 for invalid leads.
func utf8ContinuationCount(b byte) int {
	switch {
	case b&0xe0 == 0xc0:
		return 1
	case b&0xf0 == 0xe0:
		return 2
	case b&0xf8 == 0xf0:
		return 3
	}
	return 0
}
