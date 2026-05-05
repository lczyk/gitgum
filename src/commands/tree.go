package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
)

type TreeCommand struct {
	cmdIO
	Since   string   `long:"since" default:"2w" description:"limit history. shorthand: '2w', '10d', '1h' (units: s/m/h/d/w/y). ISO date: '2024-01-01'. bare integer: tree depth (last N commits). empty: show all."`
	Reverse bool     `long:"reverse" short:"r" description:"newest-first output (useful in follow mode)"`
	Follow  *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1)"`
}

var (
	shorthandRe = regexp.MustCompile(`^(\d+)([smhdwy])$`)
	isoDateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2})?)?$`)
)

var sinceUnits = map[string]string{
	"s": "seconds",
	"m": "minutes",
	"h": "hours",
	"d": "days",
	"w": "weeks",
	"y": "years",
}

// parseSinceArg interprets the --since value. Returns (sinceArg, maxCount,
// err) where exactly one of sinceArg / maxCount is non-zero (or both zero
// for "show all").
func parseSinceArg(s string) (string, int, error) {
	if s == "" {
		return "", 0, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return "", 0, fmt.Errorf("--since=%q: depth must be a positive integer", s)
		}
		return "", n, nil
	}
	if m := shorthandRe.FindStringSubmatch(s); m != nil {
		return fmt.Sprintf("%s %s ago", m[1], sinceUnits[m[2]]), 0, nil
	}
	if isoDateRe.MatchString(s) {
		return s, 0, nil
	}
	return "", 0, fmt.Errorf("--since=%q: unrecognised form. use shorthand (2w, 10d, 1h; units s/m/h/d/w/y), ISO date (2024-01-01), bare integer (depth), or empty (all)", s)
}

// TODO: replace the `git log --graph` shell-out with an in-process renderer.
// Plan: fetch raw commit/parent/ref data from git (e.g. `git log --format=...`
// + `git for-each-ref`), build the graph ourselves, and render it. Initial
// output should match this format byte-for-byte; later it gains interactive
// fuzzyfinder features. Keep tests structural (no exact-format pins) so the
// swap is local to this function. The reverse-and-flip dance below also goes
// away once we render natively — we'll just emit oldest-first directly.
func (t *TreeCommand) Execute(args []string) error {
	if err := t.repo().CheckInRepo(); err != nil {
		return err
	}
	sinceArg, maxCount, err := parseSinceArg(t.Since)
	if err != nil {
		return err
	}
	if t.Follow == nil {
		return t.renderOnce(t.out(), sinceArg, maxCount)
	}
	return t.runFollow(sinceArg, maxCount)
}

func (t *TreeCommand) renderOnce(w io.Writer, sinceArg string, maxCount int) error {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	gitArgs := []string{"log", "--graph", "--oneline", "--all", "--decorate", colorFlag}
	if sinceArg != "" {
		gitArgs = append(gitArgs, "--since", sinceArg)
	}
	if maxCount > 0 {
		gitArgs = append(gitArgs, fmt.Sprintf("-%d", maxCount))
	}

	stdout, _, runErr := t.repo().Run(gitArgs...)
	if runErr != nil {
		return fmt.Errorf("git log: %w", runErr)
	}
	if strings.TrimSpace(stdout) == "" {
		return nil
	}

	if t.Reverse {
		fmt.Fprintln(w, stdout)
	} else {
		fmt.Fprintln(w, reverseGraph(stdout))
	}
	return nil
}

func (t *TreeCommand) runFollow(sinceArg string, maxCount int) error {
	if !stdoutIsTTY() {
		return errors.New("--follow requires a tty")
	}
	interval := *t.Follow
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

	const fullRefreshEvery = 60 * time.Second
	var (
		cachedRefs   string
		cachedLines  []string
		cachedAt     time.Time
		cachedErr    error
		forceRender  = true
		scrollOffset = 0
		tailMode     = true
	)

	refreshCache := func() {
		refs, refsErr := t.snapshotRefs()
		stale := time.Since(cachedAt) > fullRefreshEvery
		if forceRender || stale || refsErr != nil || refs != cachedRefs {
			var treeBuf bytes.Buffer
			cachedErr = t.renderOnce(&treeBuf, sinceArg, maxCount)
			body := strings.Trim(treeBuf.String(), "\n")
			if body == "" {
				cachedLines = nil
			} else {
				cachedLines = strings.Split(body, "\n")
			}
			cachedRefs = refs
			cachedAt = time.Now()
			forceRender = false
		}
	}

	redraw := func() {
		w, h := scr.Size()
		scr.Clear()

		dimStyle := tcell.StyleDefault.Dim(true)
		ts := time.Now().Format("15:04:05")
		header := fmt.Sprintf("last update: %s -- interval %.1fs (j/k g/G scroll, q exit)", ts, interval)
		writePlain(scr, 0, 0, header, dimStyle, w, h)

		visible := max(h-1, 0)

		if cachedErr != nil {
			errStyle := tcell.StyleDefault.Foreground(tcell.PaletteColor(1))
			writePlain(scr, 0, 1, "git error: "+cachedErr.Error(), errStyle, w, h)
			scr.Show()
			return
		}

		maxOffset := max(len(cachedLines)-visible, 0)
		if tailMode {
			scrollOffset = maxOffset
		}
		scrollOffset = max(0, min(scrollOffset, maxOffset))
		end := min(scrollOffset+visible, len(cachedLines))
		for i, line := range cachedLines[scrollOffset:end] {
			writeAnsi(scr, 0, 1+i, line, tcell.StyleDefault, w, h)
		}
		scr.Show()
	}

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
				if !handleFollowKey(ev, &scrollOffset, &tailMode, h) {
					return nil
				}
				redraw()
			}
		}
	}
}

// handleFollowKey applies a key event to the follow-mode scroll state.
// Returns false when the key requests exit. screenH is the current screen
// height; used for page-size math.
func handleFollowKey(ev *tcell.EventKey, scrollOffset *int, tailMode *bool, screenH int) bool {
	page := max(screenH-2, 1) // leave one line of context on page nav
	switch {
	case ev.Key() == tcell.KeyCtrlC, ev.Key() == tcell.KeyEscape, ev.Rune() == 'q':
		return false
	case ev.Rune() == 'j', ev.Key() == tcell.KeyDown:
		*scrollOffset++
		*tailMode = false
	case ev.Rune() == 'k', ev.Key() == tcell.KeyUp:
		*scrollOffset--
		*tailMode = false
	case ev.Rune() == 'g', ev.Key() == tcell.KeyHome:
		*scrollOffset = 0
		*tailMode = false
	case ev.Rune() == 'G', ev.Key() == tcell.KeyEnd:
		*tailMode = true
	case ev.Key() == tcell.KeyPgDn, ev.Key() == tcell.KeyCtrlD, ev.Rune() == ' ':
		*scrollOffset += page
		*tailMode = false
	case ev.Key() == tcell.KeyPgUp, ev.Key() == tcell.KeyCtrlU:
		*scrollOffset -= page
		*tailMode = false
	}
	return true
}

// writePlain writes a single-line string into the screen at (x0, y), clipping
// at the terminal width / height. No ansi parsing.
func writePlain(scr *litescreen.Screen, x0, y int, s string, style tcell.Style, w, h int) {
	if y < 0 || y >= h {
		return
	}
	x := x0
	for _, r := range s {
		if x >= w {
			break
		}
		scr.SetContent(x, y, r, nil, style)
		x++
	}
}

// writeAnsi writes a possibly-multiline ANSI-escaped string into the screen
// starting at (x0, y0), advancing newline-by-newline. Lines that exceed
// terminal width or height are clipped.
func writeAnsi(scr *litescreen.Screen, x0, y0 int, s string, base tcell.Style, w, h int) {
	set := func(x, y int, r rune, style tcell.Style) {
		if y < 0 || y >= h || x < 0 || x >= w {
			return
		}
		scr.SetContent(x, y, r, nil, style)
	}
	ansi.WriteToScreen(set, x0, y0, s, base)
}

// snapshotRefs returns a cheap fingerprint of all refs + HEAD. Used to skip
// the expensive `git log --graph` shell-out when the repo hasn't changed.
func (t *TreeCommand) snapshotRefs() (string, error) {
	refs, _, err := t.repo().Run("for-each-ref", "--format=%(objectname) %(refname)")
	if err != nil {
		return "", err
	}
	head, _, err := t.repo().Run("rev-parse", "HEAD")
	if err != nil {
		return refs, err
	}
	return refs + "\n" + head, nil
}

// reverseGraph flips git's --graph output so the tip lands at the bottom
// (right above the next prompt). git refuses --graph + --reverse, so we
// reverse line order and swap '/' <-> '\' in each line's graph prefix to
// keep diagonal connectors pointing the right way after the y-axis flip.
func reverseGraph(out string) string {
	lines := strings.Split(out, "\n")
	slices.Reverse(lines)
	for i, line := range lines {
		lines[i] = swapGraphSlashes(line)
	}
	return strings.Join(lines, "\n")
}

// swapGraphSlashes swaps '/' and '\' in the graph-drawing prefix of a line.
// The prefix is everything before the first letter or digit (which would
// belong to the hash or subject), skipping over ANSI SGR escapes since
// `--color=always` wraps the graph chars themselves in color codes.
func swapGraphSlashes(line string) string {
	end := 0
	for end < len(line) {
		c := line[end]
		if c == 0x1b && end+1 < len(line) && line[end+1] == '[' {
			// skip ANSI SGR: ESC [ ... m
			end += 2
			for end < len(line) && line[end] != 'm' {
				end++
			}
			if end < len(line) {
				end++ // consume the 'm'
			}
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			break
		}
		end++
	}
	if end == 0 {
		return line
	}
	swapped := strings.Map(func(r rune) rune {
		switch r {
		case '/':
			return '\\'
		case '\\':
			return '/'
		}
		return r
	}, line[:end])
	return swapped + line[end:]
}
