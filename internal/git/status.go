package git

import (
	"context"
	"fmt"
	"strings"
)

// UntrackedMode selects how much of an untracked directory git reports.
type UntrackedMode int

const (
	// UntrackedNormal reports an entirely-untracked directory as a single
	// entry and stops walking there. Cheap on a tree with a fat node_modules,
	// and already folded -- which is what a report wants.
	UntrackedNormal UntrackedMode = iota
	// UntrackedAll names every untracked file. What a caller about to delete
	// them needs: a directory standing in for its contents hides the contents.
	UntrackedAll
)

// ScanOpts is what a caller wants out of one status read.
type ScanOpts struct {
	Untracked UntrackedMode
	Ignored   bool // include ignored files, individually
	Branch    bool // include the leading "## ..." line
}

// Entry is one porcelain v1 record: the two status characters and the path
// they describe. RenamedFrom is the source half of a rename or copy, empty
// otherwise -- both halves matter, since a hard reset restores the source as
// well as removing the destination.
type Entry struct {
	X, Y        byte
	Path        string
	RenamedFrom string
}

// Untracked reports whether git does not know about this path.
func (e Entry) Untracked() bool { return e.X == '?' }

// Ignored reports whether .gitignore covers this path.
func (e Entry) Ignored() bool { return e.X == '!' }

// Staged reports whether the index differs from HEAD for this path.
func (e Entry) Staged() bool { return known(e.X) }

// Unstaged reports whether the working tree differs from the index.
func (e Entry) Unstaged() bool { return known(e.Y) }

// known distinguishes a real status character from the ones that mean "no
// change on this side" (space) or "not a tracked file at all" (? and !).
func known(c byte) bool { return c != ' ' && c != '?' && c != '!' }

// StatusArgs builds the invocation for one scan. It is the only place flags
// are chosen, so two callers asking for different scans cannot end up
// disagreeing about what a scan means.
func StatusArgs(opt ScanOpts) []string {
	// -z because git C-quotes paths in line-based output: a filename with a
	// space comes back wrapped in quotes, which a caller would then try to
	// delete literally.
	args := []string{"status", "--porcelain", "-z"}
	if opt.Branch {
		args = append(args, "--branch")
	}
	// Stated rather than defaulted: status.showUntrackedFiles can move the
	// default under us, and the two modes differ in whether a directory hides
	// its contents.
	switch opt.Untracked {
	case UntrackedAll:
		args = append(args, "-uall")
	default:
		args = append(args, "-unormal")
	}
	if opt.Ignored {
		// Bare --ignored, never --ignored=matching: matching collapses an
		// ignored directory to one entry, which is the collapse a caller
		// asking for ignored files is trying to see past.
		args = append(args, "--ignored")
	}
	return args
}

// Status runs one scan and parses it.
//
// It reads through runRead rather than Run because Run trims: a record for an
// unstaged change opens with a space (" M path"), and trimming the first one
// turns it into a staged change.
func (r Repo) Status(opt ScanOpts) (branch string, entries []Entry, err error) {
	stdout, stderr, err := r.runRead(context.Background(), StatusArgs(opt)...)
	if err != nil {
		return "", nil, fmt.Errorf("git status: %w: %s", err, strings.TrimSpace(stderr))
	}
	return ParseStatus(stdout)
}

// ParseStatus parses NUL-separated porcelain v1 output. branch comes back
// verbatim, "## " and all, or empty when the scan didn't ask for it.
//
// A record is "XY<space>path". Renames and copies spend a second record on
// their source path, destination first. An unreadable record is an error
// rather than a skip: every caller of this is deciding what to show for
// something it is about to change, and a dropped path there reports less than
// will happen.
func ParseStatus(raw string) (branch string, entries []Entry, err error) {
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		if strings.HasPrefix(f, "##") {
			branch = f
			continue
		}
		if len(f) < 4 || f[2] != ' ' {
			return "", nil, fmt.Errorf("git status: unreadable record %q", f)
		}
		e := Entry{X: f[0], Y: f[1], Path: f[3:]}
		if pairs(e.X) || pairs(e.Y) {
			if i+1 >= len(fields) || fields[i+1] == "" {
				return "", nil, fmt.Errorf("git status: record %q has no source path", f)
			}
			i++
			e.RenamedFrom = fields[i]
		}
		entries = append(entries, e)
	}
	return branch, entries, nil
}

// pairs reports whether a status character brings a second path with it.
func pairs(c byte) bool { return c == 'R' || c == 'C' }
