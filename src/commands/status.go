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
	All    bool     `long:"all" short:"a" description:"render every section, same as the 'all' section list"`
	Flat   bool     `long:"flat" description:"show changes as flat porcelain list instead of tree"`
	Follow *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1). never fetches."`
	Args   struct {
		Sections *string `positional-arg-name:"SECTIONS" description:"comma-separated list of sections to render: all (a), branch (b), changes (c), head (h), remote (r), worktree (w). default: changes,head"`
	} `positional-args:"yes"`

	// sections is resolved from the positional argument in Execute.
	sections []statusSection
	// reads is the git output the sections of one render pass share.
	// renderSections replaces it every pass, so --follow still re-reads the
	// working tree on every tick.
	reads *statusReads
	// width is the column count section rules are padded to. Zero means
	// "measure os.Stdout", which is only right when that is where the output
	// lands: --follow renders into a buffer it then paints onto a litescreen
	// of its own, usually shorter, size, so it sets this instead.
	width int
}

func (s *StatusCommand) headerWidth() int {
	if s.width > 0 {
		return s.width
	}
	return stdoutWidth()
}

// Execute renders the sections named by the sole positional argument, a
// comma-separated list like "branch,worktree" or "b,w". Defaults to "changes,head".
//
// go-flags puts that argument in Args.Sections and hands Execute whatever is
// left over, while the tests call Execute directly with a plain slice; joining
// the two back together up front means one path counts them. Sections is a
// pointer because `gg status ""` is an argument, and a rejected one -- a plain
// string could not tell that from no argument at all.
func (s *StatusCommand) Execute(args []string) error {
	if err := s.repo().CheckInRepo(); err != nil {
		return err
	}
	if s.Args.Sections != nil {
		args = append([]string{*s.Args.Sections}, args...)
	}
	if len(args) > 1 {
		return fmt.Errorf("status takes at most one section list, got %d arguments", len(args))
	}
	spec := strings.Join(defaultSections, ",")
	if len(args) == 1 {
		spec = args[0]
	}
	if s.All {
		if len(args) == 1 {
			return fmt.Errorf("--all and a section list (%q) are mutually exclusive", args[0])
		}
		spec = allSections
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

// statusHeader renders a section rule padded out to width columns; a
// non-positive width falls back to 80.
func statusHeader(label string, width int) string {
	prefix := "- " + label + " "
	if width <= 0 {
		width = 80
	}
	pad := max(width-len(prefix), 3)
	return prefix + strings.Repeat("-", pad)
}

// stdoutWidth reports the terminal width of os.Stdout, or 0 when it isn't a
// terminal (piped output, test buffer).
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
