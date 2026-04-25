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
	"strings"
	"sync"

	"github.com/lczyk/gitgum/src/fuzzyfinder"
	"github.com/lczyk/gitgum/src/version"
)

const (
	exitOK        = 0
	exitNoMatch   = 1
	exitUsage     = 2
	exitCancelled = 130
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version.FormatVersion(version.Version, version.CommitSHA, version.BuildDate, version.BuildInfo))
			os.Exit(exitOK)
		}
	}
	if isTTY(os.Stdin) {
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
	multi     bool
	query     string
	prompt    string
	header    string
	selectOne bool
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	// Read synchronously until we have at least one item (or EOF). This avoids
	// launching the picker — and opening /dev/tty — when stdin is closed empty.
	br := bufio.NewReader(stdin)
	first, err := readFirstLine(br)
	if err != nil {
		fmt.Fprintf(stderr, "fuzzyfinder: read stdin: %v\n", err)
		return exitUsage
	}
	if first == "" {
		return exitNoMatch
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		lock  sync.Mutex
		items = []string{first}
	)
	readErrCh := make(chan error, 1)
	go func() { readErrCh <- streamItems(ctx, br, &lock, &items) }()

	// build picker options
	opts := []fuzzyfinder.Option{fuzzyfinder.WithPromptString(cfg.prompt)}
	if cfg.query != "" {
		opts = append(opts, fuzzyfinder.WithQuery(cfg.query))
	}
	if cfg.header != "" {
		opts = append(opts, fuzzyfinder.WithHeader(cfg.header))
	}
	if cfg.selectOne {
		opts = append(opts, fuzzyfinder.WithSelectOne())
	}
	opts = append(opts, fuzzyfinder.WithContext(ctx))

	var (
		idxs    []int
		findErr error
	)
	if cfg.multi {
		idxs, findErr = fuzzyfinder.FindMultiLive(&items, &lock, opts...)
	} else {
		idx, err := fuzzyfinder.FindLive(&items, &lock, opts...)
		idxs, findErr = []int{idx}, err
	}
	cancel()

	if err := <-readErrCh; err != nil {
		fmt.Fprintf(stderr, "fuzzyfinder: read stdin: %v\n", err)
		return exitUsage
	}

	lock.Lock()
	defer lock.Unlock()
	if len(items) == 0 {
		return exitNoMatch
	}

	// write results
	if findErr != nil {
		if errors.Is(findErr, fuzzyfinder.ErrAbort) {
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
// EOF without yielding one. Trailing \r is trimmed.
func readFirstLine(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		line = strings.TrimRight(strings.TrimSuffix(line, "\n"), "\r")
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
// EOF or ctx is cancelled.
func streamItems(ctx context.Context, r io.Reader, lock *sync.Mutex, items *[]string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
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
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: fuzzyfinder [flags]")
		fmt.Fprintln(stderr, "Reads items from stdin (one per line), writes selection to stdout.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}

	var cfg config
	fs.BoolVar(&cfg.multi, "m", false, "multi-select (shorthand)")
	fs.BoolVar(&cfg.multi, "multi", false, "multi-select")
	fs.StringVar(&cfg.query, "q", "", "initial query (shorthand)")
	fs.StringVar(&cfg.query, "query", "", "initial query")
	fs.StringVar(&cfg.prompt, "p", "> ", "prompt prefix (shorthand)")
	fs.StringVar(&cfg.prompt, "prompt", "> ", "prompt prefix")
	fs.StringVar(&cfg.header, "header", "", "static header line")
	fs.BoolVar(&cfg.selectOne, "1", false, "auto-select if exactly one item (shorthand)")
	fs.BoolVar(&cfg.selectOne, "select-1", false, "auto-select if exactly one item")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	return cfg, nil
}

