// Package dirty holds gg's answer to two questions: what is in the working
// tree that git would not keep, and what discarding it would destroy.
//
// It sits below the command layer because both callers need the same answer
// for different reasons -- `gg clean` exists to discard, while the stash
// prompt in `gg switch`, `gg pull` and the rest offers discarding as one of
// three ways past a dirty tree. Reporting a set smaller than the one about to
// be destroyed is the failure mode this package exists to prevent, so the
// groups and their counts are the point, not a detail of presentation.
package dirty

import (
	"fmt"
	"strings"

	"github.com/lczyk/gitgum/internal/git"
)

// Repo is the slice of git this package needs. It is an interface only so the
// grouping can be exercised against canned output; git.Repo satisfies it.
//
// The scan arrives already parsed rather than as raw text, because the raw
// text has a trap in it: a record for an unstaged change opens with a space,
// and a trimming reader turns the first one into a staged change.
type Repo interface {
	Status(opt git.ScanOpts) (branch string, entries []git.Entry, err error)
	RunWrite(args ...string) (string, string, error)
	InProgress() (operation string, yes bool)
}

// Plan is everything a discard would destroy, kept in groups because the
// groups differ in how surprising their loss is: tracked changes are edits you
// made, untracked files are things you created, and ignored files are usually
// build output -- until one of them is the .env you cannot regenerate.
type Plan struct {
	Tracked   []string // staged or unstaged changes to tracked files
	Untracked []string // files git does not know about, excluding ignored
	Ignored   []string // files .gitignore covers, always listed, discarded only on request

	// Codes is the porcelain code each path arrived with, for callers that
	// show one. A rename's halves get R< and R> rather than the shared code,
	// since which half you are looking at is the interesting part. Decoration
	// only: nothing here decides what a discard destroys, and a path with no
	// entry simply renders without a code.
	Codes map[string]string
}

// Options selects which groups a discard destroys. Ignored implies Untracked:
// git has no ignored-only mode -- `git clean -x` widens the untracked sweep
// rather than narrowing to it -- so asking for one without the other would
// quietly delete more than was requested.
type Options struct {
	Tracked   bool
	Untracked bool
	Ignored   bool
}

// Empty reports whether there is nothing to discard at all.
func (p Plan) Empty() bool { return p.Count(Options{true, true, true}) == 0 }

// Count is how many files the given options would destroy.
func (p Plan) Count(o Options) int {
	n := 0
	if o.Tracked {
		n += len(p.Tracked)
	}
	if o.Untracked {
		n += len(p.Untracked)
	}
	if o.Ignored {
		n += len(p.Ignored)
	}
	return n
}

// Scan reads the working tree and groups what a discard would destroy.
//
// The scan is chosen to report what will actually happen rather than what
// reads nicely. UntrackedAll names every untracked file individually, where
// the cheaper mode collapses a directory to one entry and hides its contents;
// ignored files are always listed, since whether to destroy them is the
// caller's choice and it cannot make it without seeing them.
func Scan(r Repo) (Plan, error) {
	_, entries, err := r.Status(git.ScanOpts{Untracked: git.UntrackedAll, Ignored: true})
	if err != nil {
		return Plan{}, fmt.Errorf("listing the working tree: %w", err)
	}
	return group(entries), nil
}

// group turns a scan into a plan. It is deliberately free of git so the cases
// that matter -- a file both staged and unstaged, a rename's two halves, an
// untracked path that is also reported as ignored -- can be exercised as plain
// data.
//
// Anything that is neither untracked nor ignored counts as tracked, rather
// than being matched against a list of known status characters. A record this
// package does not recognise is one it would otherwise drop, and a plan that
// lists less than the discard destroys is the failure it exists to prevent.
func group(entries []git.Entry) Plan {
	p := Plan{Codes: map[string]string{}}
	seen := map[string]bool{}
	add := func(dst *[]string, path, code string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		p.Codes[path] = code
		*dst = append(*dst, path)
	}
	for _, e := range entries {
		code := string(e.X) + string(e.Y)
		switch {
		case e.Untracked():
			add(&p.Untracked, e.Path, code)
		case e.Ignored():
			add(&p.Ignored, e.Path, code)
		case e.RenamedFrom != "":
			// Both halves: a hard reset removes the destination and restores
			// the source.
			add(&p.Tracked, e.Path, "R>")
			add(&p.Tracked, e.RenamedFrom, "R<")
		default:
			// A file that is staged and unstaged both is one file at risk,
			// which the dedup handles.
			add(&p.Tracked, e.Path, code)
		}
	}
	return p
}

// Discard destroys the groups the options select.
//
// It refuses outright when an operation owns the working tree. A hard reset
// mid-merge ends the merge as a side effect, which is a far larger thing than
// this function claims to do, and no confirmation shown beforehand would have
// described it.
func (p Plan) Discard(r Repo, o Options) error {
	if operation, yes := r.InProgress(); yes {
		return fmt.Errorf("%s is in progress; finish or abort it first, then discard", operation)
	}
	if o.Ignored {
		o.Untracked = true
	}
	if o.Tracked && len(p.Tracked) > 0 {
		if _, stderr, err := r.RunWrite("reset", "--hard"); err != nil {
			return fmt.Errorf("discarding tracked changes: %w: %s", err, strings.TrimSpace(stderr))
		}
	}
	if o.Untracked && (len(p.Untracked) > 0 || len(p.Ignored) > 0) {
		if _, stderr, err := r.RunWrite(cleanArgs(o.Ignored)...); err != nil {
			return fmt.Errorf("removing untracked files: %w: %s", err, strings.TrimSpace(stderr))
		}
	}
	return nil
}

// cleanArgs builds the removal invocation. -x widens the sweep to ignored
// files; without it they are left alone.
func cleanArgs(ignored bool) []string {
	args := []string{"clean", "-fd"}
	if ignored {
		args = append(args, "-x")
	}
	return args
}
