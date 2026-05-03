package git

import (
	"context"
	"fmt"
	"strings"
)

// Commit creates a commit with the given message. Output streams live so
// user sees pre-commit / commit-msg hook output. User signing config
// (commit.gpgsign, gpg.program) is honoured because runWrite preserves
// user identity. The captured stderr tail is included in error messages.
func (r Repo) Commit(message string) error {
	if _, stderr, err := r.runWriteStreaming(context.Background(), "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

// CommitEmpty is like Commit but adds --allow-empty so the commit succeeds
// even when the index has no changes.
func (r Repo) CommitEmpty(message string) error {
	if _, stderr, err := r.runWriteStreaming(context.Background(), "commit", "--allow-empty", "-m", message); err != nil {
		return fmt.Errorf("git commit --allow-empty: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

func Commit(message string) error      { return CWD().Commit(message) }
func CommitEmpty(message string) error { return CWD().CommitEmpty(message) }
