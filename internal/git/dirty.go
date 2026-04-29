package git

import (
	"bytes"
	"fmt"
	"os/exec"
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
	// Use raw exec — cmdrun.RunIn trims output, which would eat the leading
	// space of " M file" entries.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git status: %w: %s", err, stderr.String())
	}
	if stdout.Len() == 0 {
		return nil, nil
	}
	var dirty []string
	for _, line := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
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

// StashPush stashes tracked changes (staged + unstaged) under the given
// message. Untracked files are not included.
func (r Repo) StashPush(message string) error {
	_, _, err := r.run("stash", "push", "-m", message)
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
	_, _, err := r.run("stash", "pop", "--index")
	if err != nil {
		return fmt.Errorf("git stash pop --index: %w", err)
	}
	return nil
}

func DirtyTrackedLines() ([]string, error) { return CWD().DirtyTrackedLines() }
func StashPush(message string) error       { return CWD().StashPush(message) }
func StashPopIndex() error                 { return CWD().StashPopIndex() }
