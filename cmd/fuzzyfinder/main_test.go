package main

import (
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
	err := streamItems(context.Background(), strings.NewReader("a\nb\r\n\nc\n"), &lock, &items, 0)
	assert.NoError(t, err)
	want := []string{"a", "b", "c"}
	assert.EqualArrays(t, items, want)
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
