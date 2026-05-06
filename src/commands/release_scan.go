package commands

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/ui"
)

// versionMention is a single line in a tracked manifest/lockfile that
// references the current project version. The release command prints these
// as a heads-up so the user can bump downstream files (Cargo.toml,
// package.json, ...) by hand before re-running the release.
type versionMention struct {
	path string // relative to repo root
	line int
	text string
}

// scanFileMaxBytes is the per-file size cap. Files larger than this are
// skipped -- generated lockfiles, vendored bundles, etc. shouldn't usually
// be scanned and would slow the picker. 4 MiB covers all realistic
// hand-edited manifests + medium lockfiles.
const scanFileMaxBytes = 4 << 20

// binaryProbeBytes is how much of a file's head we sniff to decide whether
// it's binary. Any null byte in this window means binary -> skip.
const binaryProbeBytes = 8192

// lineMentionsVersion is the single line-context rule: line contains both
// the word "version" (case-insensitive) and a boundaried occurrence of the
// version string. Wide enough to cover python `__version__`, go
// `const version =`, yaml `version:`, json `"version":`, autotools
// `PYTHON_VERSION`, and arbitrary comments / docstrings; narrow enough that
// random version-like numbers in unrelated lines don't flood the picker.
//
// The remaining false positives (changelog entries, comments mentioning a
// past version) are filtered by the user via the multi-select picker.
func lineMentionsVersion(line, version string) bool {
	if !containsVersionToken(line, version) {
		return false
	}
	return strings.Contains(strings.ToLower(line), "version")
}

// scanVersionMentions returns lines in tracked manifest/lockfiles that both
// reference the literal current version (with digit/dot boundary) and
// contain the word "version" (case-insensitive). The VERSION file itself
// is skipped -- it's handled by the bumper directly.
func scanVersionMentions(r git.Repo, root, current string) ([]versionMention, error) {
	out, _, err := r.Run("ls-files")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var mentions []versionMention
	for rel := range strings.SplitSeq(out, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" || rel == versionFileName {
			continue
		}
		ms, err := scanFileForVersion(filepath.Join(root, rel), rel, current)
		if err != nil {
			continue
		}
		mentions = append(mentions, ms...)
	}
	return mentions, nil
}

// scanFileForVersion returns version-mention lines in absPath. Files that
// are missing, too large, or detected as binary return (nil, nil) -- a soft
// skip, not an error. Read errors return (nil, err).
func scanFileForVersion(absPath, relPath, current string) ([]versionMention, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > scanFileMaxBytes {
		return nil, nil
	}
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	probe := make([]byte, binaryProbeBytes)
	pn, err := io.ReadFull(f, probe)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if bytes.IndexByte(probe[:pn], 0) >= 0 {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var out []versionMention
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		if !lineMentionsVersion(line, current) {
			continue
		}
		out = append(out, versionMention{path: relPath, line: n, text: strings.TrimSpace(line)})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func containsVersionToken(line, version string) bool {
	i := 0
	for {
		idx := strings.Index(line[i:], version)
		if idx == -1 {
			return false
		}
		start := i + idx
		end := start + len(version)
		leftOK := start == 0 || !isVersionTokenChar(line[start-1])
		rightOK := end == len(line) || !isVersionTokenChar(line[end])
		if leftOK && rightOK {
			return true
		}
		i = start + 1
	}
}

func isVersionTokenChar(b byte) bool {
	return (b >= '0' && b <= '9') || b == '.'
}

// pickVersionEdits asks the user which mentions (if any) the release should
// auto-update via plain string replace. Returns the chosen mentions, or nil
// if the picker was aborted (the release continues without auto-edits).
func pickVersionEdits(sel ui.Selector, mentions []versionMention, current string) ([]versionMention, error) {
	if len(mentions) == 0 {
		return nil, nil
	}
	options := make([]string, len(mentions))
	byOption := make(map[string]versionMention, len(mentions))
	for i, m := range mentions {
		opt := fmt.Sprintf("%s:%d  %s", m.path, m.line, m.text)
		options[i] = opt
		byOption[opt] = m
	}
	prompt := fmt.Sprintf(
		"Found %d mention(s) of version %s. Tab to mark for auto-update, Enter to confirm, Esc to skip",
		len(mentions), current,
	)
	selected, err := sel.MultiSelect(prompt, options)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return nil, nil
		}
		return nil, err
	}
	picks := make([]versionMention, 0, len(selected))
	for _, s := range selected {
		if m, ok := byOption[s]; ok {
			picks = append(picks, m)
		}
	}
	return picks, nil
}

// applyVersionEdits rewrites the picked lines in each file, replacing every
// occurrence of `current` with `next` on those lines. No language-specific
// parsing -- pure string replace. Returns the relative paths of files that
// were modified, in stable order.
func applyVersionEdits(root string, picks []versionMention, current, next string) ([]string, error) {
	if len(picks) == 0 {
		return nil, nil
	}
	// group picks by file
	byPath := make(map[string][]int)
	for _, p := range picks {
		byPath[p.path] = append(byPath[p.path], p.line)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, rel := range paths {
		if err := rewriteLines(filepath.Join(root, rel), byPath[rel], current, next); err != nil {
			return nil, fmt.Errorf("update %s: %w", rel, err)
		}
	}
	return paths, nil
}

// rewriteLines reads absPath, replaces `current` with `next` on every line
// whose 1-based line number is in lineNums, and writes the file back. The
// file's trailing-newline state is preserved. Other lines are untouched.
func rewriteLines(absPath string, lineNums []int, current, next string) error {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	hasTrailingNL := len(data) > 0 && data[len(data)-1] == '\n'
	body := string(data)
	if hasTrailingNL {
		body = body[:len(body)-1]
	}
	lines := strings.Split(body, "\n")
	pick := make(map[int]bool, len(lineNums))
	for _, n := range lineNums {
		pick[n] = true
	}
	for i := range lines {
		if pick[i+1] {
			lines[i] = strings.ReplaceAll(lines[i], current, next)
		}
	}
	out := strings.Join(lines, "\n")
	if hasTrailingNL {
		out += "\n"
	}
	return os.WriteFile(absPath, []byte(out), 0o644)
}
