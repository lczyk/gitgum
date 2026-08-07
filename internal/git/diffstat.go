package git

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Modes as git writes them in a raw diff record. ModeAbsent is the side of the
// pair that does not exist: a new file has it on the left, a deleted one on
// the right.
const (
	ModeAbsent  = "000000"
	ModeFile    = "100644"
	ModeExec    = "100755"
	ModeSymlink = "120000"
)

// DiffFile is one file in a diff: what happened to it, and how much of it
// changed. The two halves come from the two views git offers of the same diff
// -- raw names the modes and the kind of change, numstat counts the lines --
// and neither alone says enough to describe a row.
type DiffFile struct {
	Path string
	// RenamedFrom is the source half of a rename or copy, empty otherwise.
	RenamedFrom string
	Added       int
	Deleted     int
	// Binary marks a file git counts no lines for.
	Binary bool
	// OldMode and NewMode are ModeAbsent on the side where the file does not
	// exist, so a new file, a deleted one and a chmod are all told apart here.
	OldMode string
	NewMode string
	// Status is the raw status letter: A added, D deleted, M modified,
	// R renamed, C copied, T type-changed.
	Status byte
}

// Renamed reports whether this row describes a path that moved.
func (f DiffFile) Renamed() bool { return f.RenamedFrom != "" }

// DiffFiles describes a diff file by file. extra is passed to git verbatim --
// "--cached", a revision range, pathspecs.
//
// One invocation asks for both views: git emits the raw records first and the
// numstat records after, and a raw record is the only one that opens with ':'.
func (r Repo) DiffFiles(extra ...string) ([]DiffFile, error) {
	args := slices.Concat([]string{"diff", "--raw", "--numstat", "-z"}, extra)
	stdout, stderr, err := r.runRead(context.Background(), args...)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(stderr))
	}
	return ParseDiffFiles(stdout)
}

// ParseDiffFiles parses the combined `--raw --numstat -z` output. It is split
// out so the record shapes -- a rename spending two fields on its paths, a
// binary file counting "-" instead of a number -- can be exercised as plain
// strings.
func ParseDiffFiles(raw string) ([]DiffFile, error) {
	fields := strings.Split(raw, "\x00")
	files := map[string]*DiffFile{}
	var order []string

	i := 0
	for ; i < len(fields) && strings.HasPrefix(fields[i], ":"); i++ {
		oldMode, newMode, status, err := parseRawHeader(fields[i])
		if err != nil {
			return nil, err
		}
		f := DiffFile{OldMode: oldMode, NewMode: newMode, Status: status}
		if status == 'R' || status == 'C' {
			if i+2 >= len(fields) || fields[i+1] == "" || fields[i+2] == "" {
				return nil, fmt.Errorf("git diff: rename record %q has no paths", fields[i])
			}
			f.RenamedFrom, f.Path = fields[i+1], fields[i+2]
			i += 2
		} else {
			if i+1 >= len(fields) || fields[i+1] == "" {
				return nil, fmt.Errorf("git diff: record %q has no path", fields[i])
			}
			f.Path = fields[i+1]
			i++
		}
		files[f.Path] = &f
		order = append(order, f.Path)
	}

	for ; i < len(fields); i++ {
		if fields[i] == "" {
			continue
		}
		added, deleted, path, err := parseNumstatRecord(fields[i])
		if err != nil {
			return nil, err
		}
		if path == "" {
			// A rename spends the path field on nothing and puts source and
			// destination in the two fields after it.
			if i+2 >= len(fields) || fields[i+2] == "" {
				return nil, fmt.Errorf("git diff: numstat rename record %q has no paths", fields[i])
			}
			path = fields[i+2]
			i += 2
		}
		f, ok := files[path]
		if !ok {
			// numstat without a raw record should not happen; keeping the row
			// beats dropping a file from a report of what changed.
			f = &DiffFile{Path: path}
			files[path] = f
			order = append(order, path)
		}
		f.Added, f.Deleted, f.Binary = added, deleted, added < 0
	}

	out := make([]DiffFile, 0, len(order))
	for _, path := range order {
		f := files[path]
		if f.Binary {
			f.Added, f.Deleted = 0, 0
		}
		out = append(out, *f)
	}
	return out, nil
}

// parseRawHeader reads ":<oldmode> <newmode> <oldsha> <newsha> <status>".
// The status carries a similarity score for renames and copies ("R100"),
// which the letter alone answers for.
func parseRawHeader(s string) (oldMode, newMode string, status byte, err error) {
	parts := strings.Fields(strings.TrimPrefix(s, ":"))
	if len(parts) < 5 || parts[4] == "" {
		return "", "", 0, fmt.Errorf("git diff: unreadable raw record %q", s)
	}
	return parts[0], parts[1], parts[4][0], nil
}

// parseNumstatRecord reads "<added>\t<deleted>\t<path>". A binary file counts
// "-" on both sides, which comes back as -1 for the caller to recognise.
func parseNumstatRecord(s string) (added, deleted int, path string, err error) {
	a, rest, ok := strings.Cut(s, "\t")
	if !ok {
		return 0, 0, "", fmt.Errorf("git diff: unreadable numstat record %q", s)
	}
	d, path, ok := strings.Cut(rest, "\t")
	if !ok {
		return 0, 0, "", fmt.Errorf("git diff: unreadable numstat record %q", s)
	}
	if a == "-" || d == "-" {
		return -1, -1, path, nil
	}
	if added, err = strconv.Atoi(a); err != nil {
		return 0, 0, "", fmt.Errorf("git diff: unreadable insertion count %q", a)
	}
	if deleted, err = strconv.Atoi(d); err != nil {
		return 0, 0, "", fmt.Errorf("git diff: unreadable deletion count %q", d)
	}
	return added, deleted, path, nil
}
