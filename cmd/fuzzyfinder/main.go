// fuzzyfinder is a small fzf-like CLI built on the gitgum fuzzyfinder library.
// Items are read from stdin (one per line); the selection is written to stdout.
//
// Exit codes match fzf where reasonable: 0 success, 1 no match, 130 cancelled,
// 2 IO/flag error.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

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
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
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

	items, err := readItems(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "fuzzyfinder: read stdin: %v\n", err)
		return exitUsage
	}
	if len(items) == 0 {
		return exitNoMatch
	}

	opts := buildOptions(cfg)
	if cfg.multi {
		idxs, err := fuzzyfinder.FindMulti(items, func(i int) string { return items[i] }, opts...)
		return writeResult(stdout, stderr, items, idxs, err)
	}
	idx, err := fuzzyfinder.Find(items, func(i int) string { return items[i] }, opts...)
	return writeResult(stdout, stderr, items, []int{idx}, err)
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

func buildOptions(cfg config) []fuzzyfinder.Option {
	opts := []fuzzyfinder.Option{
		fuzzyfinder.WithPromptString(cfg.prompt),
	}
	if cfg.query != "" {
		opts = append(opts, fuzzyfinder.WithQuery(cfg.query))
	}
	if cfg.header != "" {
		opts = append(opts, fuzzyfinder.WithHeader(cfg.header))
	}
	if cfg.selectOne {
		opts = append(opts, fuzzyfinder.WithSelectOne())
	}
	return opts
}

func readItems(r io.Reader) ([]string, error) {
	var items []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line != "" {
			items = append(items, line)
		}
	}
	return items, scanner.Err()
}

func writeResult(stdout, stderr io.Writer, items []string, idxs []int, err error) int {
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return exitCancelled
		}
		fmt.Fprintf(stderr, "fuzzyfinder: %v\n", err)
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
