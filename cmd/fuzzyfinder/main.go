// fuzzyfinder is a small fzf-like CLI built on the gitgum fuzzyfinder library.
// Items are read from stdin (one per line); the selection is written to stdout.
//
// Exit codes match fzf where reasonable: 0 success, 1 no match, 130 cancelled,
// 2 IO/flag error.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lczyk/gitgum/src/completions"
	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
	"github.com/lczyk/gitgum/src/litescreen/ansi"
	vinfo "github.com/lczyk/gitgum/src/version"
	ver "github.com/lczyk/version/go"
)

const (
	exitOK        = 0
	exitNoMatch   = 1
	exitUsage     = 2
	exitCancelled = 130
)

// streamDelay matches gg switch's per-item throttle so the picker animates
// items in at the same cadence regardless of producer.
const streamDelay = 3 * time.Millisecond

func main() {
	hasCompletion := false
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(ver.FormatVersion(vinfo.Version, vinfo.CommitSHA, vinfo.BuildDate, vinfo.BuildInfo))
			os.Exit(exitOK)
		}
		if arg == "--help" || arg == "-h" {
			printUsage(os.Stdout)
			os.Exit(exitOK)
		}
		if arg == "--completion" || arg == "-completion" || strings.HasPrefix(arg, "--completion=") || strings.HasPrefix(arg, "-completion=") {
			hasCompletion = true
		}
	}
	if !hasCompletion && isTTY(os.Stdin) {
		fmt.Fprintln(os.Stderr, "fuzzyfinder: stdin is a terminal; pipe input via stdin (e.g. `find . | fuzzyfinder`)")
		os.Exit(exitUsage)
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// isTTY reports whether f is connected to a terminal (no piped/redirected input).
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type config struct {
	opt        ff.Opt
	fast       bool
	completion string
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	if cfg.completion != "" {
		out, err := completions.RenderFuzzyfinder(cfg.completion, filepath.Base(os.Args[0]))
		if err != nil {
			fmt.Fprintf(stderr, "fuzzyfinder: %v\n", err)
			return exitUsage
		}
		fmt.Fprint(stdout, out)
		return exitOK
	}

	// Read synchronously until we have at least one item (or EOF). This avoids
	// launching the picker — and opening /dev/tty — when stdin is closed empty.
	br := bufio.NewReader(stdin)
	first, err := readFirstLine(br, !cfg.opt.Ansi)
	if err != nil {
		fmt.Fprintf(stderr, "fuzzyfinder: read stdin: %v\n", err)
		return exitUsage
	}
	if first == "" {
		return exitNoMatch
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If stdin is piped from a producer that may also write to our
	// terminal's stderr (the classic `find ~ | fuzzyfinder` case — find's
	// "permission denied" lines tear the picker), force full repaints so
	// the next frame overwrites the garbage. Skipped when stderr isn't a
	// TTY: the producer's stderr is being captured elsewhere, no tearing.
	// Tests pass a non-*os.File stdin; the cast guards against that.
	if stdinFile, ok := stdin.(*os.File); ok && !isTTY(stdinFile) && isTTY(os.Stderr) {
		cfg.opt.RedrawAggressive = true
	}

	var (
		lock  sync.Mutex
		items = []string{first}
	)
	readErrCh := make(chan error, 1)
	delay := streamDelay
	if cfg.fast {
		delay = 0
	}
	go func() { readErrCh <- streamItems(ctx, br, &lock, &items, delay, !cfg.opt.Ansi) }()

	idxs, findErr := ff.Find(ctx, &items, &lock, cfg.opt)
	cancel()

	if err := <-readErrCh; err != nil {
		fmt.Fprintf(stderr, "fuzzyfinder: read stdin: %v\n", err)
		return exitUsage
	}

	lock.Lock()
	defer lock.Unlock()

	if findErr != nil {
		if errors.Is(findErr, ff.ErrAbort) {
			return exitCancelled
		}
		fmt.Fprintf(stderr, "fuzzyfinder: %v\n", findErr)
		return exitUsage
	}
	for _, idx := range idxs {
		if idx < 0 || idx >= len(items) {
			return exitNoMatch
		}
		fmt.Fprintln(stdout, items[idx])
	}
	return exitOK
}

// readFirstLine returns the first non-empty line from r, or "" if r reaches
// EOF without yielding one. Trailing \r is trimmed and ANSI escapes are
// stripped unless stripAnsi is false.
func readFirstLine(r *bufio.Reader, stripAnsi bool) (string, error) {
	for {
		line, err := r.ReadString('\n')
		line = strings.TrimRight(strings.TrimSuffix(line, "\n"), "\r")
		if stripAnsi {
			line = ansi.Strip(line)
		}
		if line != "" {
			return line, nil
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
}

// streamItems reads lines from r and appends them to *items under lock until
// EOF or ctx is cancelled. ANSI escapes are stripped on ingest unless
// stripAnsi is false (i.e. caller wants the picker to render colour via
// Opt.Ansi downstream).
func streamItems(ctx context.Context, r io.Reader, lock *sync.Mutex, items *[]string, delay time.Duration, stripAnsi bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if stripAnsi {
			line = ansi.Strip(line)
		}
		if line == "" {
			continue
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		lock.Lock()
		*items = append(*items, line)
		lock.Unlock()
	}
	return scanner.Err()
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("fuzzyfinder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }

	var cfg config
	fs.BoolVar(&cfg.opt.Multi, "m", false, "multi-select (shorthand)")
	fs.BoolVar(&cfg.opt.Multi, "multi", false, "multi-select")
	fs.StringVar(&cfg.opt.Query, "q", "", "initial query (shorthand)")
	fs.StringVar(&cfg.opt.Query, "query", "", "initial query")
	fs.StringVar(&cfg.opt.Prompt, "p", "> ", "prompt prefix (shorthand)")
	fs.StringVar(&cfg.opt.Prompt, "prompt", "> ", "prompt prefix")
	fs.StringVar(&cfg.opt.Header, "header", "", "static header line")
	fs.BoolVar(&cfg.opt.SelectOne, "1", false, "auto-select if exactly one item (shorthand)")
	fs.BoolVar(&cfg.opt.SelectOne, "select-1", false, "auto-select if exactly one item")
	fs.BoolVar(&cfg.fast, "fast", false, "disable streaming delay (append items as fast as stdin produces them)")
	fs.BoolVar(&cfg.opt.Reverse, "reverse", false, "render prompt at the top with items growing downward")
	fs.IntVar(&cfg.opt.Height, "height", 0, "occupy only N rows of the terminal instead of fullscreen; preserves prior terminal output above the picker. 0 = fullscreen, N>0 = exact rows, N<0 = terminal_rows + N")
	fs.BoolVar(&cfg.opt.Ansi, "ansi", false, "render ANSI SGR colour escapes from input items in the picker (default: strip)")
	fs.StringVar(&cfg.completion, "completion", "", "print shell completion script for the given shell (bash, fish, zsh, or nu) and exit")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	return cfg, nil
}

const usageText = `Usage:
  fuzzyfinder [OPTIONS]

Description:
  Interactive fzf-like fuzzy picker built on the gitgum fuzzyfinder library.
  Reads items from stdin (one per line) and writes the selected item(s) to
  stdout. The terminal UI is drawn on /dev/tty so stdout stays clean and
  can be piped into another command.

  Items stream into the picker as they arrive — pipes that take a while to
  produce all their output (find, fd, ripgrep, ...) are usable immediately,
  and you can begin typing a query before the producer has finished.

  Matching is substring-based and case-insensitive: whitespace-split queries
  require every word to appear in the item (in any order). Matched characters
  are highlighted. Empty lines from stdin are skipped.

Options:
  -m, --multi          Allow selecting multiple items. Tab toggles the item
                       under the cursor; Enter prints all selected items, one
                       per line, in selection order.
  -q, --query <s>      Start with the picker's query pre-filled to <s>. Useful
                       for narrowing results immediately or for scripting.
  -p, --prompt <s>     Prompt prefix shown before the query (default "> ").
                       Set per-context to remind the user what they are picking,
                       e.g. -p 'branch> '.
      --header <s>     Static header line displayed above the items. Does not
                       scroll and is not searched.
  -1, --select-1       Auto-select if there is exactly one matching item, with
                       no UI shown. Combine with --query for non-interactive use.
      --fast           Disable the streaming delay. Items are appended as fast
                       as stdin produces them, instead of throttled to match
                       the gg switch animation cadence.
      --reverse        Render the prompt at the top with items growing
                       downward. Default is bottom-up (prompt at the bottom).
      --height=N       Occupy only N rows of the terminal instead of going
                       fullscreen. Prior terminal output remains visible
                       above the picker.
                         0     fullscreen (default)
                         N>0   exactly N rows
                         N<0   terminal_rows + N
      --completion <s> Print a shell completion script for <s> and exit.
                       Supported shells: bash, fish, zsh, nu. Source the
                       output from your shell init.
  -v, --version        Print version and exit.
  -h, --help           Show this help message.

Key bindings:
  Enter                  Confirm selection
  Esc, Ctrl-C, Ctrl-D    Cancel (exit 130)
  Tab                    Toggle selection (with --multi)
  Up,    Ctrl-K, Ctrl-P  Move cursor up
  Down,  Ctrl-J, Ctrl-N  Move cursor down
  PgUp,  Ctrl-B          Page up
  PgDn,  Ctrl-F          Page down
  Left                   Move query cursor left
  Right                  Move query cursor right
  Home,  Ctrl-A          Jump to start of query
  End,   Ctrl-E          Jump to end of query
  Backspace / Delete     Delete char before / under cursor
  Ctrl-W                 Delete previous word
  Ctrl-U                 Clear query
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}
