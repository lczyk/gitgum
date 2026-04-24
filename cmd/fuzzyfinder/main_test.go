package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

func TestParseFlags(t *testing.T) {
	cfg, err := parseFlags([]string{"-m", "-q", "foo", "--prompt", "$ ", "--header", "h", "-1"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !cfg.multi || cfg.query != "foo" || cfg.prompt != "$ " || cfg.header != "h" || !cfg.selectOne {
		t.Fatalf("flags not applied: %+v", cfg)
	}
}

func TestParseFlags_DefaultPrompt(t *testing.T) {
	cfg, err := parseFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.prompt != "> " {
		t.Fatalf("default prompt = %q, want %q", cfg.prompt, "> ")
	}
}

func TestParseFlags_BadFlag(t *testing.T) {
	_, err := parseFlags([]string{"--no-such-flag"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for bad flag")
	}
}

func TestStreamItems(t *testing.T) {
	var (
		lock  sync.Mutex
		items []string
	)
	if err := streamItems(context.Background(), strings.NewReader("a\nb\r\n\nc\n"), &lock, &items); err != nil {
		t.Fatalf("streamItems: %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(items) != len(want) {
		t.Fatalf("items = %v, want %v", items, want)
	}
	for i, v := range want {
		if items[i] != v {
			t.Fatalf("items[%d] = %q, want %q", i, items[i], v)
		}
	}
}

func TestRun_EmptyStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader(""), &stdout, &stderr)
	if code != exitNoMatch {
		t.Fatalf("exit code = %d, want %d", code, exitNoMatch)
	}
}

func TestRun_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "Usage") {
		t.Fatalf("stderr should contain usage; got %q", stderr.String())
	}
}

func TestRun_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-such-flag"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
}
