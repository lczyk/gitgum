package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

func TestContainsVersionToken(t *testing.T) {
	tests := []struct {
		line, version string
		want          bool
	}{
		{`version = "0.7.1"`, "0.7.1", true},
		{`"version": "0.7.1",`, "0.7.1", true},
		{`version = "0.7.10"`, "0.7.1", false}, // boundary -- 0.7.10
		{`version = "10.0.7.1"`, "0.7.1", false},
		{`# 0.7.1 here`, "0.7.1", true},
		{`v0.7.1`, "0.7.1", true},
		{`0.7.1`, "0.7.1", true},
		{`tag = "rust/v0.5.0"`, "0.7.1", false},
		{``, "0.7.1", false},
		{`0.7.10.7.1`, "0.7.1", false}, // both occurrences boundary-fail
	}
	for _, tt := range tests {
		assert.Equal(t, containsVersionToken(tt.line, tt.version), tt.want)
	}
}

func TestLineMentionsVersion(t *testing.T) {
	tests := []struct {
		line, version string
		want          bool
	}{
		{`version = "0.7.1"`, "0.7.1", true},
		{`"version": "0.7.1",`, "0.7.1", true},
		{`Version: 0.7.1`, "0.7.1", true},                             // case-insensitive
		{`__version__ = "0.7.1"`, "0.7.1", true},                      // python style
		{`const version = "0.7.1"`, "0.7.1", true},                    // go style
		{`# tag 0.7.1`, "0.7.1", false},                               // no "version" word
		{`version = "0.7.10"`, "0.7.1", false},                        // boundary fail
		{`version = { git = "...", tag = "v0.5.0" }`, "0.7.1", false}, // version word but no token match
	}
	for _, tt := range tests {
		assert.Equal(t, lineMentionsVersion(tt.line, tt.version), tt.want)
	}
}

func TestScanFileForVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	body := `[package]
name = "edit"
version = "0.7.1"  # source: /VERSION

[dependencies]
version = { git = "https://example.com", tag = "rust/v0.5.0" }
serde = "0.7.1"
`
	err := os.WriteFile(path, []byte(body), 0o644)
	require.NoError(t, err)

	got, err := scanFileForVersion(path, "Cargo.toml", "0.7.1")
	require.NoError(t, err)
	// only the package version line matches: contains both "version" and 0.7.1.
	// the "serde = 0.7.1" line has the token but not the word "version".
	// the "version = { ..., tag = ... }" line has "version" but not the token.
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].path, "Cargo.toml")
	assert.Equal(t, got[0].line, 3)
}

func TestScanFileForVersion_Missing(t *testing.T) {
	_, err := scanFileForVersion("/nonexistent/file", "x", "1.0.0")
	assert.Error(t, err, assert.AnyError)
}

func TestScanFileForVersion_BinarySkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	// Null byte in the probe window -> treated as binary, soft-skip.
	body := append([]byte("version = \"1.0.0\"\n\x00"), make([]byte, 100)...)
	err := os.WriteFile(path, body, 0o644)
	require.NoError(t, err)

	got, err := scanFileForVersion(path, "blob", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, len(got), 0)
}

func TestScanFileForVersion_LargeFileSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge")
	// One byte over the cap -> soft-skip.
	body := make([]byte, scanFileMaxBytes+1)
	for i := range body {
		body[i] = 'a'
	}
	copy(body, []byte("version = \"1.0.0\"\n"))
	err := os.WriteFile(path, body, 0o644)
	require.NoError(t, err)

	got, err := scanFileForVersion(path, "huge", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, len(got), 0)
}

func TestRewriteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cargo.toml")
	body := `[package]
name = "edit"
version = "0.7.1"  # comment

[dependencies]
serde = "0.7.1"
`
	err := os.WriteFile(path, []byte(body), 0o644)
	require.NoError(t, err)

	// Only rewrite line 3.
	err = rewriteLines(path, []int{3}, "0.7.1", "0.7.2")
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	want := `[package]
name = "edit"
version = "0.7.2"  # comment

[dependencies]
serde = "0.7.1"
`
	assert.Equal(t, string(got), want)
}

func TestRewriteLines_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	err := os.WriteFile(path, []byte(`version = "1.0.0"`), 0o644)
	require.NoError(t, err)

	err = rewriteLines(path, []int{1}, "1.0.0", "1.0.1")
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(got), `version = "1.0.1"`)
}

func TestApplyVersionEdits(t *testing.T) {
	root := t.TempDir()
	cargo := filepath.Join(root, "Cargo.toml")
	err := os.WriteFile(cargo, []byte("version = \"1.0.0\"\nother = \"1.0.0\"\n"), 0o644)
	require.NoError(t, err)

	pkg := filepath.Join(root, "package.json")
	err = os.WriteFile(pkg, []byte("{\n  \"version\": \"1.0.0\"\n}\n"), 0o644)
	require.NoError(t, err)

	picks := []versionMention{
		{path: "Cargo.toml", line: 1},
		{path: "package.json", line: 2},
	}
	paths, err := applyVersionEdits(root, picks, "1.0.0", "1.0.1")
	require.NoError(t, err)
	assert.EqualArrays(t, paths, []string{"Cargo.toml", "package.json"})

	got, _ := os.ReadFile(cargo)
	assert.Equal(t, string(got), "version = \"1.0.1\"\nother = \"1.0.0\"\n")
	got, _ = os.ReadFile(pkg)
	assert.Equal(t, string(got), "{\n  \"version\": \"1.0.1\"\n}\n")
}

func TestApplyVersionEdits_Empty(t *testing.T) {
	paths, err := applyVersionEdits(t.TempDir(), nil, "1.0.0", "1.0.1")
	require.NoError(t, err)
	assert.Equal(t, len(paths), 0)
}

func TestPickVersionEdits_NoMentions(t *testing.T) {
	stub := &stubSelector{}
	picks, err := pickVersionEdits(stub, nil, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, len(picks), 0)
	assert.Equal(t, len(stub.multiSelectCalls), 0)
}

func TestPickVersionEdits_PickSubset(t *testing.T) {
	mentions := []versionMention{
		{path: "Cargo.toml", line: 3, text: `version = "1.0.0"`},
		{path: "package.json", line: 2, text: `"version": "1.0.0",`},
	}
	want := `Cargo.toml:3  version = "1.0.0"`
	stub := &stubSelector{multiSelectAnswers: [][]string{{want}}}

	picks, err := pickVersionEdits(stub, mentions, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, len(picks), 1)
	assert.Equal(t, picks[0].path, "Cargo.toml")
	assert.Equal(t, picks[0].line, 3)
}

func TestPickVersionEdits_PickNothing(t *testing.T) {
	mentions := []versionMention{{path: "Cargo.toml", line: 3, text: `version = "1.0.0"`}}
	stub := &stubSelector{multiSelectAnswers: [][]string{{}}}

	picks, err := pickVersionEdits(stub, mentions, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, len(picks), 0)
}
