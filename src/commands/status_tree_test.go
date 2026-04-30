package commands

import (
	"regexp"
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestParseChangeLines(t *testing.T) {
	cases := map[string]struct {
		input    []string
		expected []changeEntry
	}{
		"empty": {input: []string{}, expected: nil},
		"single modified": {
			input:    []string{" M go.mod"},
			expected: []changeEntry{{code: " M", path: "go.mod"}},
		},
		"untracked": {
			input:    []string{"?? delete_me"},
			expected: []changeEntry{{code: "??", path: "delete_me"}},
		},
		"rename splits into two": {
			input: []string{"R  old/path.go -> new/path.go"},
			expected: []changeEntry{
				{code: "R<", path: "old/path.go"},
				{code: "R>", path: "new/path.go"},
			},
		},
		"skips short lines": {
			input:    []string{"", "x"},
			expected: nil,
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			got := parseChangeLines(tt.input)
			assert.EqualArrays(t, got, tt.expected)
		})
	}
}

func TestRenderTree_Flat(t *testing.T) {
	entries := []changeEntry{
		{code: " D", path: "go.mod"},
		{code: "??", path: "delete_me"},
	}
	var buf strings.Builder
	renderTree(buildTree(entries), &buf)
	got := stripAnsi(buf.String())
	// alpha sort: delete_me before go.mod
	expected := "" +
		"[??] delete_me\n" +
		"[ D] go.mod\n"
	assert.EqualLineByLine(t, got, expected)
}

func TestRenderTree_Nested(t *testing.T) {
	entries := []changeEntry{
		{code: " M", path: "internal/git/git.go"},
		{code: "??", path: "src/commands/foo.go"},
		{code: " D", path: "go.mod"},
	}
	var buf strings.Builder
	renderTree(buildTree(entries), &buf)
	got := stripAnsi(buf.String())
	expected := "" +
		"[ D] go.mod\n" +
		"internal/\n" +
		"└─ git/\n" +
		"   └─ [ M] git.go\n" +
		"src/\n" +
		"└─ commands/\n" +
		"   └─ [??] foo.go\n"
	assert.EqualLineByLine(t, got, expected)
}

func TestRenderTree_RenameTwoLeaves(t *testing.T) {
	entries := parseChangeLines([]string{"R  a/old.go -> b/new.go"})
	var buf strings.Builder
	renderTree(buildTree(entries), &buf)
	got := stripAnsi(buf.String())
	assert.ContainsString(t, got, "[R<] old.go")
	assert.ContainsString(t, got, "[R>] new.go")
}
