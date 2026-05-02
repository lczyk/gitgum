package git

import (
	"context"
	"fmt"
	"strings"
)

// DirtyTrackedLines returns porcelain v1 status lines for tracked changes
// (staged or unstaged). Untracked entries ("?? ...") are filtered out so
// callers can decide whether the working tree is dirty in a way that
// matters for operations like stash + release.
//
// Lines are returned verbatim including the two-char XY status code, so
// callers can distinguish ` M` (unstaged), `M ` (staged), and `MM`
// (partial-hunk staging).
func (r Repo) DirtyTrackedLines() ([]string, error) {
	stdout, stderr, err := r.runRead(context.Background(), "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w: %s", err, stderr)
	}
	if stdout == "" {
		return nil, nil
	}
	var dirty []string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if strings.HasPrefix(line, "?? ") {
			continue
		}
		if line == "" {
			continue
		}
		dirty = append(dirty, line)
	}
	return dirty, nil
}

// stashHooksOff suppresses user pre-stash / post-checkout hooks for the
// duration of a stash op. gg uses stash internally only (release auto-
// stash, switch_stream bookkeeping); firing user hooks on plumbing they
// didn't initiate is a footgun.
var stashHooksOff = []string{"-c", "core.hooksPath=/dev/null"}

// StashPush stashes tracked changes (staged + unstaged) under the given
// message. Untracked files are not included.
func (r Repo) StashPush(message string) error {
	args := append(stashHooksOff, "stash", "push", "-m", message)
	_, _, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git stash push: %w", err)
	}
	return nil
}

// StashPopIndex pops the most recent stash with --index, restoring the
// exact staged-vs-unstaged split (including partial-hunk staging). On
// conflict, git leaves the stash entry in place; the caller should treat
// that as a manual-resolution situation rather than retrying.
func (r Repo) StashPopIndex() error {
	args := append(stashHooksOff, "stash", "pop", "--index")
	_, _, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git stash pop --index: %w", err)
	}
	return nil
}

func DirtyTrackedLines() ([]string, error) { return CWD().DirtyTrackedLines() }
func StashPush(message string) error       { return CWD().StashPush(message) }
func StashPopIndex() error                 { return CWD().StashPopIndex() }
