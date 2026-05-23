package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
)

// diffModes is the set of modes addressable in auto cascade and in the
// follow tab loop. "untracked" is intentionally not in this set -- it is
// opt-in only via --mode untracked, never shown by default.
var diffModes = []string{"work", "index", "head"}

// untrackedCountTimeout caps per-file line-counting for untracked entries.
// On timeout the entry renders with `+???,-???` and a `+++---` bar marker
// rather than blocking the whole diff render.
const untrackedCountTimeout = 200 * time.Millisecond

type DiffCommand struct {
	cmdIO
	Follow *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1)"`
	Mode   string   `long:"mode" short:"m" description:"lock to a diff level: work (unstaged), index (staged), head (last commit), untracked (opt-in, not shown by default). default: auto-cascade over work/index/head"`
}

func (d *DiffCommand) Execute(args []string) error {
	if err := d.repo().CheckInRepo(); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("diff takes no arguments")
	}
	if d.Mode != "" && d.Mode != "work" && d.Mode != "index" && d.Mode != "head" && d.Mode != "untracked" {
		return fmt.Errorf("--mode must be one of: work, index, head, untracked")
	}
	if d.Follow != nil {
		return d.runFollow()
	}
	return d.render(d.out())
}

func (d *DiffCommand) render(w io.Writer) error {
	if d.Mode == "untracked" {
		return d.renderUntracked(w)
	}
	out, level, err := d.collectOutput()
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	fmt.Fprintln(w, dim("--- "+level+" ---"))
	if os.Getenv("GG_DIFF_NATIVE") == "1" {
		return d.renderCollected(w, out)
	}
	fmt.Fprintln(w, out)
	return nil
}

func (d *DiffCommand) renderUntracked(w io.Writer) error {
	entries, err := d.collectUntrackedEntries()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	fmt.Fprintln(w, dim("--- untracked ---"))
	renderTree(buildTree(entries), w)
	return nil
}

func (d *DiffCommand) collectUntrackedEntries() ([]changeEntry, error) {
	out, _, err := d.repo().Run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	paths := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	var entries []changeEntry
	for _, p := range paths {
		if p == "" {
			continue
		}
		full := filepath.Join(d.repo().Dir, p)
		ns := countUntrackedLines(full, untrackedCountTimeout)
		entries = append(entries, changeEntry{code: "??", path: p, numstat: &ns})
	}
	return entries, nil
}

// countUntrackedLines reads path with a hard deadline. It detects binary
// content via NUL in the first 8 KiB and otherwise counts `\n` plus a
// trailing partial line. On timeout / read error it returns an `unknown`
// numstat -- the renderer surfaces that as `+???,-???` with a `+++---` bar.
func countUntrackedLines(path string, timeout time.Duration) numstat {
	ch := make(chan numstat, 1)
	go func() {
		ch <- doCountUntrackedLines(path)
	}()
	select {
	case n := <-ch:
		return n
	case <-time.After(timeout):
		return numstat{unknown: true}
	}
}

func doCountUntrackedLines(path string) numstat {
	f, err := os.Open(path)
	if err != nil {
		return numstat{unknown: true}
	}
	defer f.Close()
	const sniff = 8 * 1024
	head := make([]byte, sniff)
	n, _ := f.Read(head)
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return numstat{binary: true}
	}
	lines := bytes.Count(head, []byte{'\n'})
	var lastByte byte
	if n > 0 {
		lastByte = head[n-1]
	}
	buf := make([]byte, 32*1024)
	for {
		nn, rerr := f.Read(buf)
		if nn > 0 {
			lines += bytes.Count(buf[:nn], []byte{'\n'})
			lastByte = buf[nn-1]
		}
		if rerr != nil {
			break
		}
	}
	if lastByte != 0 && lastByte != '\n' {
		lines++
	}
	return numstat{added: lines}
}

func (d *DiffCommand) collectDiff(level string) (string, error) {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	// Repo.Run TrimSpaces stdout, which eats the leading space git
	// --compact-summary emits on every row. Restore it so the first row
	// aligns with the rest.
	restore := func(s string) string {
		if s == "" {
			return s
		}
		return " " + s
	}
	switch level {
	case "work":
		out, _, err := d.repo().Run("diff", "--compact-summary", colorFlag)
		if err != nil {
			return "", fmt.Errorf("git diff: %w", err)
		}
		return restore(out), nil
	case "index":
		out, _, err := d.repo().Run("diff", "--cached", "--compact-summary", colorFlag)
		if err != nil {
			return "", fmt.Errorf("git diff --cached: %w", err)
		}
		return restore(out), nil
	case "head":
		out, _, err := d.repo().Run("diff", "--compact-summary", colorFlag, "HEAD~1..HEAD")
		if err != nil {
			return "", nil
		}
		return restore(out), nil
	case "untracked":
		entries, err := d.collectUntrackedEntries()
		if err != nil {
			return "", err
		}
		if len(entries) == 0 {
			return "", nil
		}
		var buf bytes.Buffer
		renderTree(buildTree(entries), &buf)
		return strings.TrimRight(buf.String(), "\n"), nil
	default:
		return "", fmt.Errorf("unknown diff level: %s", level)
	}
}

// collectOutput runs the cascade (work -> index -> head) unless Mode is set,
// in which case it returns that level directly. returns (output, level, error).
func (d *DiffCommand) collectOutput() (string, string, error) {
	if d.Mode != "" {
		out, err := d.collectDiff(d.Mode)
		return out, d.Mode, err
	}
	for _, level := range diffModes {
		out, err := d.collectDiff(level)
		if err != nil {
			return "", level, err
		}
		if out != "" {
			return out, level, nil
		}
	}
	return "", "", nil
}

var emptyModeMessages = map[string]string{
	"work":      "(no work changes)",
	"index":     "(no index changes)",
	"head":      "(no commits)",
	"untracked": "(no untracked)",
}

func (d *DiffCommand) runFollow() error {
	if !stdoutIsTTY() {
		return errors.New("--follow requires a tty")
	}
	interval := *d.Follow
	if interval < 1.0 {
		interval = 1.0
	}

	scr, err := litescreen.New(0)
	if err != nil {
		return fmt.Errorf("init screen: %w", err)
	}
	if err := scr.Init(); err != nil {
		return fmt.Errorf("init screen: %w", err)
	}
	defer scr.Fini()

	events := make(chan tcell.Event)
	quit := make(chan struct{})
	go scr.ChannelEvents(events, quit)
	defer close(quit)

	tick := time.NewTicker(time.Duration(interval * float64(time.Second)))
	defer tick.Stop()

	// primary: the mode tab cycles from. shown with <> braces.
	// pinned: modes that stay visible across tab switches. shown bold.
	// 1/2/3: set primary, clear all pins.
	// tab: advance primary to next, clear all pins.
	// shift+tab: pin current primary, advance primary to next.
	// !/@ /#: toggle pin on that specific mode.
	primaryMode := d.Mode
	if primaryMode == "" {
		for _, level := range diffModes {
			out, err := d.collectDiff(level)
			if err == nil && out != "" {
				primaryMode = level
				break
			}
		}
		if primaryMode == "" {
			primaryMode = "work"
		}
	}
	pinned := map[string]bool{}

	nextMode := func(from string) string {
		for i, m := range diffModes {
			if m == from {
				return diffModes[(i+1)%len(diffModes)]
			}
		}
		return diffModes[0]
	}

	isActive := func(m string) bool {
		return m == primaryMode || pinned[m]
	}

	activeCount := func() int {
		n := 1
		for _, m := range diffModes {
			if m != primaryMode && pinned[m] {
				n++
			}
		}
		return n
	}

	var (
		cachedLines  []string
		cachedErr    error
		scrollOffset = 0
		tailMode     = true
	)

	refreshCache := func() {
		cachedErr = nil
		cachedLines = nil
		multi := activeCount() > 1
		first := true
		for _, m := range diffModes {
			if !isActive(m) {
				continue
			}
			out, cErr := d.collectDiff(m)
			if cErr != nil {
				cachedErr = cErr
				cachedLines = nil
				return
			}
			if !first && multi {
				cachedLines = append(cachedLines, "")
			}
			if multi {
				cachedLines = append(cachedLines, ansiDim+"--- "+m+" ---"+ansiReset)
			}
			first = false
			body := strings.Trim(out, "\n")
			if body == "" {
				cachedLines = append(cachedLines, emptyModeMessages[m])
			} else {
				cachedLines = append(cachedLines, strings.Split(body, "\n")...)
			}
		}
	}

	frame := newFollowFrame(scr)
	redraw := func() {
		frame.Begin()
		frame.Header(interval, "", "j/k g/G tab q")
		w, h := scr.Size()
		dimStyle := tcell.StyleDefault.Dim(true)
		boldDimStyle := tcell.StyleDefault.Bold(true).Dim(true)
		frame.ExtraRow(func(y int) {
			x := 0
			for i, m := range diffModes {
				if i > 0 {
					writePlain(scr, x, y, " ", dimStyle, w, h)
					x++
				}
				n := i + 1
				var label string
				var style tcell.Style
				switch {
				case m == primaryMode:
					label = fmt.Sprintf("<%d:%s>", n, m)
					style = boldDimStyle
				case pinned[m]:
					label = fmt.Sprintf("%d:%s", n, m)
					style = boldDimStyle
				default:
					label = fmt.Sprintf("%d:%s", n, m)
					style = dimStyle
				}
				writePlain(scr, x, y, label, style, w, h)
				x += len(label)
			}
		})
		frame.Body(cachedLines, &scrollOffset, &tailMode, cachedErr)
		frame.End()
	}

	shiftedNum := map[rune]string{'!': "work", '@': "index", '#': "head"}

	refreshCache()
	redraw()
	for {
		select {
		case <-tick.C:
			refreshCache()
			redraw()
		case ev := <-events:
			switch ev := ev.(type) {
			case *tcell.EventResize:
				redraw()
			case *tcell.EventKey:
				_, h := scr.Size()
				switch {
				case ev.Rune() == '1':
					primaryMode = "work"
					clear(pinned)
					refreshCache()
				case ev.Rune() == '2':
					primaryMode = "index"
					clear(pinned)
					refreshCache()
				case ev.Rune() == '3':
					primaryMode = "head"
					clear(pinned)
					refreshCache()
				case ev.Key() == tcell.KeyTab:
					clear(pinned)
					primaryMode = nextMode(primaryMode)
					refreshCache()
				case ev.Key() == tcell.KeyBacktab:
					pinned[primaryMode] = true
					primaryMode = nextMode(primaryMode)
					refreshCache()
				case shiftedNum[ev.Rune()] != "":
					// TODO: primary-fallover UX when shift+number deselects
					// the active primary feels clunky. revisit.
					m := shiftedNum[ev.Rune()]
					if m == primaryMode && activeCount() > 1 {
						delete(pinned, m)
						for _, candidate := range diffModes {
							if pinned[candidate] {
								primaryMode = candidate
								delete(pinned, candidate)
								break
							}
						}
					} else if pinned[m] {
						delete(pinned, m)
					} else {
						pinned[m] = true
					}
					refreshCache()
				default:
					if !handleFollowKey(ev, &scrollOffset, &tailMode, h) {
						return nil
					}
				}
				redraw()
			}
		}
	}
}
