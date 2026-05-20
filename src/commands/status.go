package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	Follow *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1). suppresses branches/remotes; never fetches."`
}

func (s *StatusCommand) Execute(args []string) error {
	if err := s.repo().CheckInRepo(); err != nil {
		return err
	}
	if s.Follow == nil {
		return s.renderFull(s.out())
	}
	return s.runFollow()
}

// renderFull writes the standard four-section status: branches, remotes,
// changes, status. Used by the non-follow path.
func statusHeader(label string) string {
	prefix := "- " + label + " "
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		w = 80
	}
	pad := w - len(prefix)
	if pad < 3 {
		pad = 3
	}
	return prefix + strings.Repeat("-", pad)
}

func (s *StatusCommand) renderFull(out io.Writer) error {
	printHeader := func(label string) {
		fmt.Fprintln(out, paint(ansiDim, statusHeader(label)))
	}

	printHeader("BRANCHES")
	stdout, _, err := s.repo().Run("branch", "-vv", "--color=never")
	if err != nil {
		return fmt.Errorf("getting branches: %w", err)
	}
	fmt.Fprintln(out, renderBranchList(stdout))

	stdout, _, err = s.repo().Run("remote", "-v")
	if err != nil {
		return fmt.Errorf("getting remotes: %w", err)
	}
	remotes := parseRemotes(stdout)
	if len(remotes) > 0 {
		printHeader("REMOTES")
		for _, remote := range remotes {
			fmt.Fprintln(out, remote)
		}
	}

	return s.renderBody(out)
}

// renderBody writes only the CHANGES + STATUS sections. Shared between the
// non-follow render and the follow-loop redraw. Runs `git status` only --
// no fetch, no remote ops.
func (s *StatusCommand) renderBody(out io.Writer) error {
	printHeader := func(label string) {
		fmt.Fprintln(out, paint(ansiDim, statusHeader(label)))
	}

	stdout, _, err := s.repo().Run("status", "--short", "--branch")
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}
	lines := strings.Split(stdout, "\n")

	changeLines := lines[1:]
	hasChanges := false
	for _, l := range changeLines {
		if l != "" {
			hasChanges = true
			break
		}
	}
	if hasChanges {
		printHeader("CHANGES")
		if s.Flat {
			fmt.Fprintln(out, strings.Join(changeLines, "\n"))
		} else {
			entries := parseChangeLines(changeLines)
			annotateNumstats(s.repo(), entries)
			renderTree(buildTree(entries), out)
		}
	}

	printHeader("STATUS")
	fmt.Fprintln(out, lines[0])
	return nil
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
		var buf bytes.Buffer
		cachedErr = s.renderBody(&buf)
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
