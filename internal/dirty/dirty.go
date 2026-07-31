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
)

// Repo is the slice of git this package needs. It is an interface only so the
// grouping can be exercised against canned output; git.Repo satisfies it.
type Repo interface {
	Run(args ...string) (string, string, error)
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
// The commands are chosen to report what will actually happen rather than what
// reads nicely. `ls-files --others` names every untracked file individually,
// where `clean -n` collapses a directory to one line and hides its contents;
// `--no-renames` reports both halves of a staged rename, where the default
// names only the destination even though a hard reset restores the source too.
func Scan(r Repo) (Plan, error) {
	unstaged, err := lines(r, "diff", "--name-only", "-z")
	if err != nil {
		return Plan{}, fmt.Errorf("listing modified files: %w", err)
	}
	staged, err := lines(r, "diff", "--cached", "--name-only", "--no-renames", "-z")
	if err != nil {
		return Plan{}, fmt.Errorf("listing staged files: %w", err)
	}
	untracked, err := lines(r, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Plan{}, fmt.Errorf("listing untracked files: %w", err)
	}
	ignored, err := lines(r, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return Plan{}, fmt.Errorf("listing ignored files: %w", err)
	}
	return group(unstaged, staged, untracked, ignored), nil
}

// group turns raw listings into a plan. It is deliberately free of git so the
// cases that matter -- a file both staged and unstaged, a rename's two halves,
// an untracked path that is also reported as ignored -- can be exercised as
// plain data.
func group(unstaged, staged, untracked, ignored []string) Plan {
	seen := map[string]bool{}
	dedup := func(in []string) []string {
		var out []string
		for _, p := range in {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
		return out
	}
	// Tracked first and as one group: a file with both staged and unstaged
	// changes is one file at risk, not two.
	tracked := dedup(append(append([]string{}, unstaged...), staged...))
	return Plan{
		Tracked:   tracked,
		Untracked: dedup(untracked),
		Ignored:   dedup(ignored),
	}
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

// lines runs a NUL-delimited listing and splits it. NUL rather than newline
// because git C-quotes paths in line-based output -- a filename containing a
// space comes back wrapped in quotes, which a naive reader would then try to
// delete literally.
func lines(r Repo, args ...string) ([]string, error) {
	stdout, stderr, err := r.Run(args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
	}
	var out []string
	for _, p := range strings.Split(stdout, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
