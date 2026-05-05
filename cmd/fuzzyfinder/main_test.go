package main

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/lczyk/assert"
)

func TestParseFlags(t *testing.T) {
	cfg, err := parseFlags([]string{"-m", "-q", "foo", "--prompt", "$ ", "--header", "h", "-1"}, &bytes.Buffer{})
	assert.NoError(t, err)
	assert.That(t, cfg.opt.Multi)
	assert.Equal(t, cfg.opt.Query, "foo")
	assert.Equal(t, cfg.opt.Prompt, "$ ")
	assert.Equal(t, cfg.opt.Header, "h")
	assert.That(t, cfg.opt.SelectOne)
}

func TestParseFlags_DefaultPrompt(t *testing.T) {
	cfg, err := parseFlags(nil, &bytes.Buffer{})
	assert.NoError(t, err)
	assert.Equal(t, cfg.opt.Prompt, "> ")
}

func TestParseFlags_BadFlag(t *testing.T) {
	_, err := parseFlags([]string{"--no-such-flag"}, &bytes.Buffer{})
	assert.Error(t, err, assert.AnyError)
}

func TestStreamItems(t *testing.T) {
	var (
		lock  sync.Mutex
		items []string
	)
	err := streamItems(context.Background(), strings.NewReader("a\nb\r\n\nc\n"), &lock, &items, 0, true)
	assert.NoError(t, err)
	want := []string{"a", "b", "c"}
	assert.EqualArrays(t, items, want)
}

func TestStreamItems_StripsAnsi(t *testing.T) {
	var (
		lock  sync.Mutex
		items []string
	)
	input := "\x1b[31mred\x1b[0m\nplain\n\x1b[1;32mboldgreen\x1b[m\n"
	err := streamItems(context.Background(), strings.NewReader(input), &lock, &items, 0, true)
	assert.NoError(t, err)
	want := []string{"red", "plain", "boldgreen"}
	assert.EqualArrays(t, items, want)
}

func TestReadFirstLine_StripsAnsi(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\x1b[31mred\x1b[0m\nplain\n"))
	got, err := readFirstLine(r, true)
	assert.NoError(t, err)
	assert.Equal(t, got, "red")
}

func TestReadFirstLine_KeepsAnsiWhenStripDisabled(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\x1b[31mred\x1b[0m\n"))
	got, err := readFirstLine(r, false)
	assert.NoError(t, err)
	assert.That(t, strings.Contains(got, "\x1b["), "expected raw escape preserved, got %q", got)
}

func TestParseFlags_Ansi(t *testing.T) {
	cfg, err := parseFlags([]string{"--ansi"}, &bytes.Buffer{})
	assert.NoError(t, err)
	assert.That(t, cfg.opt.Ansi, "Ansi should be true")
}

func TestStreamItems_KeepsAnsiWhenStripDisabled(t *testing.T) {
	var (
		lock  sync.Mutex
		items []string
	)
	input := "\x1b[31mred\x1b[0m\n"
	err := streamItems(context.Background(), strings.NewReader(input), &lock, &items, 0, false)
	assert.NoError(t, err)
	assert.Equal(t, len(items), 1)
	assert.That(t, strings.Contains(items[0], "\x1b["), "expected raw escape preserved")
}

func TestRun_EmptyStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, code, exitNoMatch)
}

func TestRun_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, code, exitOK)
	assert.ContainsString(t, stderr.String(), "Usage")
}

func TestRun_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-such-flag"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, code, exitUsage)
}
