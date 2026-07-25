package commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
	"golang.org/x/term"
)

type StatusCommand struct {
	cmdIO
	Flat   bool     `long:"flat" description:"show changes as flat porcelain list instead of tree"`
	Follow *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1). never fetches."`

	// sections is resolved from the positional argument in Execute.
	sections []statusSection
	// width is the column count section rules are padded to. Zero means
	// "measure os.Stdout"; --follow sets it to the litescreen's width so the
	// rules match the surface they're actually painted on.
	width int
}

// headerWidth is the column count for section rules: an explicit width when
// one was set (follow mode), otherwise the live terminal width.
func (s *StatusCommand) headerWidth() int {
	if s.width > 0 {
		return s.width
	}
	return stdoutWidth()
}

// Execute renders the sections named by the sole positional argument, a
// comma-separated list like "branch,worktree" or "b,w". Defaults to "changes,head".
func (s *StatusCommand) Execute(args []string) error {
	if err := s.repo().CheckInRepo(); err != nil {
		return err
	}
	if len(args) > 1 {
		return fmt.Errorf("status takes at most one section list, got %d arguments", len(args))
	}
	spec := strings.Join(defaultSections, ",")
	if len(args) == 1 {
		spec = args[0]
	}
	var err error
	if s.sections, err = parseSections(spec); err != nil {
		return err
	}
	if s.Follow == nil {
		return s.renderSections(s.out(), s.sections)
	}
	return s.runFollow()
}

// statusHeader renders a section rule padded out to width columns. The width
// is passed in rather than read off os.Stdout here: under --follow the
// sections render into a buffer that is then painted onto a litescreen of its
// own size, and in tests they render into a buffer with no terminal at all.
// A non-positive width falls back to 80.
func statusHeader(label string, width int) string {
	prefix := "- " + label + " "
	if width <= 0 {
		width = 80
	}
	pad := max(width-len(prefix), 3)
	return prefix + strings.Repeat("-", pad)
}

// stdoutWidth reports the terminal width of os.Stdout, or 0 when it isn't a
// terminal (piped output, test buffer) and the caller should take the default.
func stdoutWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

func (s *StatusCommand) runFollow() error {
	if !stdoutIsTTY() {
		return errors.New("--follow requires a tty")
	}
	interval := *s.Follow
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

	var (
		cachedLines  []string
		cachedErr    error
		scrollOffset = 0
		tailMode     = true
	)

	// Render every tick. Working-tree edits don't show up in any cheap
	// fingerprint (porcelain v2 SHAs track index/HEAD, not the working
	// tree), so any cache keyed on a fingerprint silently misses content
	// changes -- the numstat counts would only refresh when the *set* of
	// changed files changed, not when an existing file's diff size did.
	refreshCache := func() {
		// Rules are padded to the screen we paint on, not to os.Stdout: the
		// litescreen band is usually shorter than the terminal.
		s.width, _ = scr.Size()
		var buf bytes.Buffer
		cachedErr = s.renderSections(&buf, s.sections)
		body := strings.Trim(buf.String(), "\n")
		cachedLines = nil
		if body != "" {
			cachedLines = strings.Split(body, "\n")
		}
	}

	frame := newFollowFrame(scr)
	redraw := func() {
		frame.Begin()
		frame.Header(interval, "interval", "j/k g/G scroll, q exit")
		frame.Body(cachedLines, &scrollOffset, &tailMode, cachedErr)
		frame.End()
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

func parseRemotes(remoteOutput string) []string {
	lines := strings.Split(remoteOutput, "\n")
	seen := make(map[string]bool)
	var remotes []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entry := fields[0] + " " + fields[1]
			if !seen[entry] {
				seen[entry] = true
				remotes = append(remotes, entry)
			}
		}
	}

	return remotes
}
