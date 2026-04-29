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
	"errors"
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

// hooks bundles every OS-side dependency the renderer touches. Production
// wires these to /dev/tty + term + os/signal in realHooks(); tests pass
// stubs (e.g. testHooks) so Init/handleResize/Fini run end-to-end without
// touching real terminals or the global signal table. Embedded into Screen
// so call sites read as `s.enterRaw()` etc. rather than `s.hooks.enterRaw()`.
type hooks struct {
	enterRaw    func() (restore func(), err error)
	getSize     func() (int, int)
	queryRow    func() int // 1-indexed cursor row at Init time; 0 = unknown (fall back to bottom-anchored layout)
	closeIO     func()
	notifyWinch func(chan<- os.Signal)
	notifySig   func(chan<- os.Signal)
	stopSignal  func(chan<- os.Signal)
	raiseSignal func(os.Signal)
}

// realHooks wires hooks to real /dev/tty + term + os/signal calls. The fd
// is captured once; in/out are closed by closeIO.
func realHooks(in *os.File, out *os.File) hooks {
	fd := int(in.Fd())
	return hooks{
		enterRaw: func() (func(), error) {
			st, err := term.MakeRaw(fd)
			if err != nil {
				return nil, err
			}
			return func() { term.Restore(fd, st) }, nil
		},
		getSize: func() (int, int) {
			w, h, err := term.GetSize(fd)
			if err != nil || w <= 0 || h <= 0 {
				return 80, 24
			}
			return w, h
		},
		queryRow:    func() int { return queryCursorRow(in, out) },
		closeIO:     func() { in.Close(); out.Close() },
		notifyWinch: func(ch chan<- os.Signal) { signal.Notify(ch, syscall.SIGWINCH) },
		notifySig:   func(ch chan<- os.Signal) { signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM) },
		stopSignal:  signal.Stop,
		raiseSignal: func(sig os.Signal) {
			signal.Reset(sig.(syscall.Signal))
			syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))
		},
	}
}

// inputSource abstracts readLoop's view of input. Production wraps /dev/tty
// with SetNonblock + EAGAIN-poll semantics; tests can swap in any reader
// (with or without simulated EAGAIN) without needing a real *os.File.
//
// Lifecycle: setup once before reads, teardown once after. read returns
// errNoData when no bytes are available right now (caller sleeps and
// retries) or any other error to signal end-of-stream.
type inputSource interface {
	setup() error
	teardown()
	read(buf []byte) (int, error)
}

// errNoData is the sentinel inputSource.read returns when no bytes are
// ready right now. Distinct from real errors so readLoop can poll instead
// of exiting.
var errNoData = errors.New("nonblocking read: no data ready")

// fdInput wraps an *os.File with raw nonblocking syscall.Read. Production
// uses this for /dev/tty so close(fd) interrupts the polling cleanly
// (close alone doesn't unblock a stranded blocking read on darwin).
//
// The fd is captured once in setup and reused in read. *os.File.Fd() must
// NOT be called on every read: each call invokes pfd.SetBlocking(), which
// clears O_NONBLOCK and turns syscall.Read back into a blocking call —
// stranding readLoop until kernel data arrives.
type fdInput struct {
	f  *os.File
	fd int
}

func (i *fdInput) setup() error {
	i.fd = int(i.f.Fd())
	return syscall.SetNonblock(i.fd, true)
}
func (i *fdInput) teardown() {
	syscall.SetNonblock(i.fd, false)
}
func (i *fdInput) read(buf []byte) (int, error) {
	n, err := syscall.Read(i.fd, buf)
	if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
		return 0, errNoData
	}
	return n, err
}

// readerInput wraps any io.Reader. Setup/teardown are no-ops; read
// delegates directly. Suitable for tests with bounded readers (e.g.
// strings.Reader); production must use fdInput because a blocking
// io.Reader.Read on /dev/tty would strand readLoop on darwin.
type readerInput struct{ r io.Reader }

func (i *readerInput) setup() error                 { return nil }
func (i *readerInput) teardown()                    {}
func (i *readerInput) read(buf []byte) (int, error) { return i.r.Read(buf) }

// inputFor picks the right inputSource for a given io.Reader: fdInput for
// real *os.File, readerInput otherwise.
func inputFor(r io.Reader) inputSource {
	if f, ok := r.(*os.File); ok {
		return &fdInput{f: f}
	}
	return &readerInput{r: r}
}

type Screen struct {
	hooks // OS hooks; embedded so callers say s.enterRaw, s.getSize, etc.

	in    io.Reader   // input bytes (real /dev/tty in production, anything in tests)
	input inputSource // readLoop's view of in; lazily derived from in if nil at Init time
	out   io.Writer   // output bytes

	height  int // raw height arg to New; see resolveHeight
	yOrigin int // first row of region (0-indexed); 0 in fullscreen

	restore func() // set by Init via enterRaw, called by cleanup

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
	finiOnce    sync.Once

	// readLoop lifecycle. Init starts the goroutine; quit closes when Fini
	// wants it to stop; readDone closes when the goroutine actually exits.
	// Fini waits on readDone before closing fds — without that, darwin leaves
	// readLoop stranded in syscall.Read on the closed fd, where it later
	// steals input bytes (including DSR responses) intended for the next
	// picker. Starting readLoop in Init (caller's goroutine) instead of
	// inside ChannelEvents (a goroutine) keeps the start happens-before the
	// Fini wait, so the race detector is satisfied.
	quit     chan struct{}
	readDone chan struct{}
	bytesCh  chan byte
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

// Options configure construction of a Screen with custom IO and/or size
// reporting. Use with NewWithOptions when you need to redirect output (e.g.
// to a bytes.Buffer for tests, or a remote pty for embedding the picker in
// a larger UI). When In or Out is set, the Screen runs in "headless" mode:
// no raw mode is entered, no signal handlers are registered, and Size
// queries fall back to the user-supplied Size func (default 80x24). The
// caller is then responsible for whatever terminal management is needed.
type Options struct {
	// Height policy. See New for semantics.
	Height int
	// In is the input source. Default: opens /dev/tty.
	In io.Reader
	// Out is the output destination. Default: opens /dev/tty.
	Out io.Writer
	// Size reports terminal dimensions. Default: real ioctl on /dev/tty
	// when In/Out are unset; 80x24 when In or Out is overridden.
	Size func() (w, h int)
}

// New constructs a Screen with the given height policy:
//
//	0   fullscreen (alt-screen, preserves nothing above)
//	N>0 exactly N rows at the bottom; prior output preserved above
//	N<0 terminal_rows + N (e.g. -2 leaves 2 rows visible above)
//
// IO goes to /dev/tty. For custom IO, see NewWithOptions.
func New(height int) (*Screen, error) {
	return NewWithOptions(Options{Height: height})
}

// NewWithOptions constructs a Screen with custom IO and/or size reporting.
// If In and Out are both unset, behavior matches New — opens /dev/tty,
// real raw mode, real signal handlers. If either is set, the Screen runs
// headless (no raw mode, no signal handlers, no /dev/tty).
func NewWithOptions(opts Options) (*Screen, error) {
	headless := opts.In != nil || opts.Out != nil
	if !headless {
		in, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("open /dev/tty for read: %w", err)
		}
		out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			in.Close()
			return nil, fmt.Errorf("open /dev/tty for write: %w", err)
		}
		return &Screen{
			hooks:  realHooks(in, out),
			in:     in,
			out:    out,
			height: opts.Height,
		}, nil
	}

	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	size := opts.Size
	if size == nil {
		size = func() (int, int) { return 80, 24 }
	}
	return &Screen{
		hooks:  headlessHooks(size),
		in:     in,
		out:    out,
		height: opts.Height,
	}, nil
}

// headlessHooks returns hooks with no /dev/tty / signal side effects.
func headlessHooks(size func() (int, int)) hooks {
	return hooks{
		enterRaw:    func() (func(), error) { return func() {}, nil },
		getSize:     size,
		queryRow:    func() int { return 0 },
		closeIO:     func() {},
		notifyWinch: func(chan<- os.Signal) {},
		notifySig:   func(chan<- os.Signal) {},
		stopSignal:  func(chan<- os.Signal) {},
		raiseSignal: func(os.Signal) {},
	}
}

// queryCursorRow asks the terminal for the cursor's current row using DSR
// (\e[6n) and parses the reply (\e[Y;XR). Returns the 1-indexed row, or 0
// on error/timeout — callers fall back to bottom-anchored layout in that
// case. Called from Init before any keystroke reader is attached, so the
// reply bytes don't race with the input loop.
//
// Reads via raw syscall.Read on the fd in non-blocking mode; *os.File's
// runtime poller doesn't reliably support SetReadDeadline on /dev/tty
// opened via os.OpenFile (darwin), and a goroutine + blocking Read could
// strand a reader that later steals user keystrokes.
func queryCursorRow(in *os.File, out *os.File) int {
	fd := int(in.Fd())
	if err := syscall.SetNonblock(fd, true); err != nil {
		return 0
	}
	defer syscall.SetNonblock(fd, false)

	// Drain any pending bytes (stale DSR responses from a prior run, queued
	// keystrokes, etc.) so our parse sees only the response to the query
	// we're about to send.
	drainBuf := make([]byte, 64)
	for range 16 {
		n, err := syscall.Read(fd, drainBuf)
		if n > 0 {
			continue
		}
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK || n == 0 {
			break
		}
	}

	if _, err := out.Write([]byte("\x1b[6n")); err != nil {
		return 0
	}

	var got []byte
	buf := make([]byte, 32)
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, err := syscall.Read(fd, buf)
		if n > 0 {
			got = append(got, buf[:n]...)
			if bytes.IndexByte(got, 'R') >= 0 {
				break
			}
			continue
		}
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		if err != nil {
			return 0
		}
	}

	i := bytes.Index(got, []byte("\x1b["))
	if i < 0 {
		return 0
	}
	body := got[i+2:]
	end := bytes.IndexByte(body, 'R')
	if end < 0 {
		return 0
	}
	body = body[:end]
	semi := bytes.IndexByte(body, ';')
	if semi < 0 {
		return 0
	}
	row := 0
	for _, c := range body[:semi] {
		if c < '0' || c > '9' {
			return 0
		}
		row = row*10 + int(c-'0')
	}
	return row
}

func (s *Screen) Init() error {
	restore, err := s.enterRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	s.restore = restore

	w, termH := s.getSize()
	cursorRow := s.queryRow()
	out, yOrigin, fb := initSequence(s.height, w, termH, cursorRow)
	s.yOrigin = yOrigin
	s.fb = fb
	s.out.Write(out)
	s.cursorVisible = false

	if s.input == nil {
		s.input = inputFor(s.in)
	}
	s.quit = make(chan struct{})
	s.readDone = make(chan struct{})
	s.bytesCh = make(chan byte, 256)
	go func() {
		defer close(s.readDone)
		s.readLoop(s.bytesCh)
	}()

	s.winch = make(chan os.Signal, 1)
	s.notifyWinch(s.winch)

	s.sigCh = make(chan os.Signal, 1)
	s.notifySig(s.sigCh)
	go s.signalLoop(s.sigCh)
	return nil
}

// initSequence is the pure-byte composition for Init: given the user's
// height policy, current terminal size, and current cursor row (1-indexed,
// 0 = unknown), returns the byte stream to emit, plus the resolved yOrigin
// and a fresh framebuf. Pulled out of Init so the byte stream is testable
// without /dev/tty + raw mode + signals.
func initSequence(height, termW, termH, cursorRow int) (out []byte, yOrigin int, fb *framebuf) {
	rows, fullscreen := resolveHeight(height, termH)
	fb = newFramebuf(termW, rows)
	var buf bytes.Buffer
	if fullscreen {
		// Enter alt-screen, clear, home cursor, hide cursor.
		buf.WriteString("\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l")
		yOrigin = 0
	} else {
		// Inline: emit (rows-1) newlines from current cursor position to
		// scroll content up if needed, leaving `rows` rows free for the
		// region. Region anchors at the cursor's current row when there's
		// space below; otherwise it sticks to the bottom of the terminal.
		buf.WriteString("\x1b[?25l")
		for range rows - 1 {
			buf.WriteByte('\n')
		}
		// finalRow = where cursor lands after the newlines (1-indexed).
		// yOrigin = first row of region (0-indexed) = finalRow - rows.
		anchor := cursorRow
		if anchor <= 0 || anchor > termH {
			anchor = termH
		}
		finalRow := anchor + rows - 1
		if finalRow > termH {
			finalRow = termH
		}
		yOrigin = finalRow - rows
		fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[J", yOrigin+1)
	}
	return buf.Bytes(), yOrigin, fb
}

// signalLoop catches SIGINT/SIGTERM, restores the terminal, and re-raises
// the signal so the default handler runs (process exit with the correct
// status). Without this, a Ctrl-C delivered while the picker is up leaves
// the terminal in raw mode + cursor hidden.
//
// The channel is passed by value (not read off s.sigCh) so Fini can nil
// the field without racing with this goroutine's receive.
func (s *Screen) signalLoop(ch <-chan os.Signal) {
	sig, ok := <-ch
	if !ok {
		return
	}
	s.cleanup()
	s.raiseSignal(sig)
}

// cleanup restores the terminal to its pre-Init state. Idempotent: safe to
// call from both Fini and the signal goroutine. Mode is derived from
// yOrigin: 0 means fullscreen (alt-screen needs rmcup), nonzero means
// inline (clear region).
func (s *Screen) cleanup() {
	s.cleanupOnce.Do(func() {
		s.out.Write(finiSequence(s.yOrigin))
		if s.restore != nil {
			s.restore()
			s.restore = nil
		}
	})
}

// finiSequence is the pure-byte composition for Fini, given the yOrigin
// that was set up at Init time. yOrigin == 0 → fullscreen → leave alt-screen.
// yOrigin > 0 → inline → clear region.
func finiSequence(yOrigin int) []byte {
	if yOrigin == 0 {
		return []byte("\x1b[m\x1b[?25h\x1b[?1049l")
	}
	return []byte(fmt.Sprintf("\x1b[m\x1b[%d;1H\x1b[J\x1b[?25h", yOrigin+1))
}

// Fini is idempotent via finiOnce. We don't nil channel fields so that
// concurrent reads in ChannelEvents (e.g. `winch := s.winch`) stay race-free
// — close() on a channel doesn't mutate the field pointer, only the channel
// state. Without finiOnce, calling Fini twice would double-close.
func (s *Screen) Fini() {
	s.finiOnce.Do(func() {
		s.cleanup()

		if s.winch != nil {
			s.stopSignal(s.winch)
			close(s.winch)
		}
		if s.sigCh != nil {
			s.stopSignal(s.sigCh)
			close(s.sigCh)
		}
		// Signal readLoop to exit and wait for it BEFORE closing fds.
		// Otherwise the goroutine sits stranded in syscall.Read on darwin
		// and steals input (DSR, keystrokes) intended for the next picker.
		if s.quit != nil {
			close(s.quit)
			<-s.readDone
		}
		s.closeIO()
	})
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

// Sync forces a full repaint: every back-buffer cell is re-emitted regardless
// of whether the diff thinks it changed. Used to recover from external writes
// to the terminal (e.g. a sibling process writing to stderr while the picker
// is up — `find ~ 2>&1 | ff` style tearing). Also clears the picker's region
// before redrawing so stray bytes left in cells we don't otherwise touch are
// wiped. Cost is a few KB of ANSI per call (full grid + SGR) vs. O(diff)
// for Show; cheap enough to call on a timer.
func (s *Screen) Sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Mark every front-buffer cell as a sentinel so flush's diff treats
	// every back cell as changed and re-emits it.
	for y := 0; y < s.fb.height; y++ {
		for x := 0; x < s.fb.width; x++ {
			s.fb.front[y][x] = liteCell{mainc: -1}
		}
	}
	// Wipe the region (and anything below it in inline mode) before
	// repainting. Without this, tearing left in cells outside the picker's
	// own writes — or wrap residue from terminal scrolling — survives the
	// repaint.
	var prelude bytes.Buffer
	fmt.Fprintf(&prelude, "\x1b[%d;1H\x1b[J", s.yOrigin+1)
	s.out.Write(prelude.Bytes())
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
	// readLoop is started in Init (caller's goroutine) so its lifecycle is
	// established before any Fini wait — ChannelEvents just consumes from
	// the bytes channel Init prepared.
	bytesCh := s.bytesCh
	winch := s.winch

	for {
		select {
		case <-quit:
			return
		case <-winch:
			w, h := s.handleResize()
			select {
			case out <- tcell.NewEventResize(w, h):
			case <-quit:
				return
			}
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

// readLoop reads input bytes off s.input and forwards them to out. It MUST
// exit when s.quit closes; otherwise on darwin a stranded blocking
// syscall.Read on the closed /dev/tty fd later steals input bytes (DSR
// responses, keystrokes) from the next picker that opens /dev/tty.
//
// The polling+quit pattern is uniform across input types: the inputSource
// is responsible for whatever setup makes errNoData possible (production:
// SetNonblock(fd) + raw syscall.Read; tests: any io.Reader, with a fake
// inputSource if EAGAIN simulation is wanted).
func (s *Screen) readLoop(out chan<- byte) {
	defer close(out)
	if err := s.input.setup(); err != nil {
		return
	}
	defer s.input.teardown()

	buf := make([]byte, 64)
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		n, err := s.input.read(buf)
		if n > 0 {
			for i := range n {
				select {
				case out <- buf[i]:
				case <-s.quit:
					return
				}
			}
			continue
		}
		if errors.Is(err, errNoData) {
			// Poll interval: snappy enough that keystrokes feel responsive,
			// slack enough not to busy-spin when idle.
			select {
			case <-s.quit:
				return
			case <-time.After(2 * time.Millisecond):
			}
			continue
		}
		// EOF or any other error → exit.
		return
	}
}

func (s *Screen) handleResize() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, termH := s.getSize()
	out, yOrigin, rows := resizeSequence(s.height, w, termH, s.fb.width, s.yOrigin)
	s.yOrigin = yOrigin
	s.out.Write(out)
	s.fb.resize(w, rows)
	return w, rows
}

// resizeSequence is the pure-byte composition for handleResize. Given the
// height policy, new terminal size, previous framebuf width, and previous
// yOrigin (so a mid-screen anchor survives a spurious SIGWINCH), returns the
// byte stream + new yOrigin + new picker row count.
func resizeSequence(height, termW, termH, prevWidth, prevYOrigin int) (out []byte, yOrigin, rows int) {
	rows, fullscreen := resolveHeight(height, termH)
	var buf bytes.Buffer
	if fullscreen {
		yOrigin = 0
		buf.WriteString("\x1b[2J\x1b[H")
	} else {
		yOrigin = prevYOrigin
		if yOrigin < 0 || yOrigin+rows > termH {
			yOrigin = termH - rows
		}
		if termW < prevWidth {
			// Narrowing reflows previous picker rows into more visual
			// lines that scroll up past yOrigin, leaving wrapped tails
			// above it. We can't clear them precisely without knowing how
			// much wrap happened, so wipe the entire visible viewport.
			// Scrollback is untouched — pre-picker output stays scrollable.
			buf.WriteString("\x1b[2J")
		}
		fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[J", yOrigin+1)
	}
	return buf.Bytes(), yOrigin, rows
}

const escTimeout = 50 * time.Millisecond

// resetEscTimer stops t, drains any already-fired tick (safe per time.Timer
// docs), and resets to escTimeout. Used between bytes in multi-byte sequences.
func resetEscTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(escTimeout)
}

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
			resetEscTimer(timer)
		}
	}
}

func mapCSI(params string, final byte) tcell.Event {
	// Modifier-prefixed forms: "1;<mod>" before A/B/C/D/H/F. xterm modifier
	// codes: 2=shift, 3=alt, 5=ctrl, 6=shift+ctrl, 7=alt+ctrl, 8=all.
	mod := tcell.ModNone
	if rest, ok := strings.CutPrefix(params, "1;"); ok {
		switch rest {
		case "2":
			mod = tcell.ModShift
		case "3":
			mod = tcell.ModAlt
		case "5":
			mod = tcell.ModCtrl
		case "6":
			mod = tcell.ModShift | tcell.ModCtrl
		case "7":
			mod = tcell.ModAlt | tcell.ModCtrl
		}
	}
	switch final {
	case 'A':
		return tcell.NewEventKey(tcell.KeyUp, 0, mod)
	case 'B':
		return tcell.NewEventKey(tcell.KeyDown, 0, mod)
	case 'C':
		return tcell.NewEventKey(tcell.KeyRight, 0, mod)
	case 'D':
		return tcell.NewEventKey(tcell.KeyLeft, 0, mod)
	case 'H':
		return tcell.NewEventKey(tcell.KeyHome, 0, mod)
	case 'F':
		return tcell.NewEventKey(tcell.KeyEnd, 0, mod)
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
		resetEscTimer(timer)
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
