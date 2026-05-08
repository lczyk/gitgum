package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/gitgum/src/litescreen"
)

var diffModes = []string{"work", "index", "head"}

type DiffCommand struct {
	cmdIO
	Follow *float64 `long:"follow" short:"f" optional:"yes" optional-value:"2" description:"follow mode: refresh every N seconds (default 2, min 1)"`
	Mode   string   `long:"mode" short:"m" description:"lock to a diff level: work (unstaged), index (staged), head (last commit). default: auto-cascade"`
}

func (d *DiffCommand) Execute(args []string) error {
	if err := d.repo().CheckInRepo(); err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("diff takes no arguments")
	}
	if d.Mode != "" && d.Mode != "work" && d.Mode != "index" && d.Mode != "head" {
		return fmt.Errorf("--mode must be one of: work, index, head")
	}
	if d.Follow != nil {
		return d.runFollow()
	}
	return d.render(d.out())
}

func (d *DiffCommand) render(w io.Writer) error {
	if os.Getenv("GG_DIFF_NATIVE") == "1" {
		return d.renderNative(w)
	}
	return d.renderPassthrough(w)
}

func (d *DiffCommand) collectDiff(level string) (string, error) {
	colorFlag := "--color=never"
	if colorEnabled() {
		colorFlag = "--color=always"
	}
	switch level {
	case "work":
		out, _, err := d.repo().Run("diff", "--compact-summary", colorFlag)
		if err != nil {
			return "", fmt.Errorf("git diff: %w", err)
		}
		return out, nil
	case "index":
		out, _, err := d.repo().Run("diff", "--cached", "--compact-summary", colorFlag)
		if err != nil {
			return "", fmt.Errorf("git diff --cached: %w", err)
		}
		return out, nil
	case "head":
		out, _, err := d.repo().Run("diff", "--compact-summary", colorFlag, "HEAD~1..HEAD")
		if err != nil {
			return "", nil
		}
		return out, nil
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

func (d *DiffCommand) renderPassthrough(w io.Writer) error {
	out, _, err := d.collectOutput()
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	fmt.Fprintln(w, out)
	return nil
}

var emptyModeMessages = map[string]string{
	"work":  "(no work changes)",
	"index": "(no index changes)",
	"head":  "(no commits)",
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

	currentMode := d.Mode
	if currentMode == "" {
		for _, level := range diffModes {
			out, err := d.collectDiff(level)
			if err == nil && out != "" {
				currentMode = level
				break
			}
		}
		if currentMode == "" {
			currentMode = "work"
		}
	}

	var (
		cachedLines  []string
		cachedErr    error
		scrollOffset = 0
		tailMode     = true
	)

	refreshCache := func() {
		out, cErr := d.collectDiff(currentMode)
		cachedErr = cErr
		if cErr != nil {
			cachedLines = nil
			return
		}
		body := strings.TrimSpace(out)
		if body == "" {
			cachedLines = []string{emptyModeMessages[currentMode]}
		} else {
			cachedLines = strings.Split(body, "\n")
		}
	}

	redraw := func() {
		w, h := scr.Size()
		scr.Clear()

		dimStyle := tcell.StyleDefault.Dim(true)
		boldDimStyle := tcell.StyleDefault.Bold(true).Dim(true)

		ts := time.Now().Format("15:04:05")
		header := fmt.Sprintf("last update: %s -- %.1fs  (j/k g/G tab q)", ts, interval)
		writePlain(scr, 0, 0, header, dimStyle, w, h)

		x := 0
		for i, m := range diffModes {
			if i > 0 {
				writePlain(scr, x, 1, " ", dimStyle, w, h)
				x++
			}
			label := fmt.Sprintf("%d:%s", i+1, m)
			if m == currentMode {
				label = fmt.Sprintf("[%d:%s]", i+1, m)
				writePlain(scr, x, 1, label, boldDimStyle, w, h)
			} else {
				writePlain(scr, x, 1, label, dimStyle, w, h)
			}
			x += len(label)
		}

		visible := max(h-2, 0)

		if cachedErr != nil {
			errStyle := tcell.StyleDefault.Foreground(tcell.PaletteColor(1))
			writePlain(scr, 0, 2, "git error: "+cachedErr.Error(), errStyle, w, h)
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
			writeAnsi(scr, 0, 2+i, line, tcell.StyleDefault, w, h)
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
				switch {
				case ev.Rune() == '1':
					currentMode = "work"
					refreshCache()
				case ev.Rune() == '2':
					currentMode = "index"
					refreshCache()
				case ev.Rune() == '3':
					currentMode = "head"
					refreshCache()
				case ev.Key() == tcell.KeyTab:
					for i, m := range diffModes {
						if m == currentMode {
							currentMode = diffModes[(i+1)%len(diffModes)]
							break
						}
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
