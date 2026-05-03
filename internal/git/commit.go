package git

import (
	"context"
	"fmt"
)

// CommitEmpty creates an empty commit with the given message. User signing
// config (commit.gpgsign, gpg.program) is honoured because runWrite preserves
// user identity.
func (r Repo) CommitEmpty(message string) error {
	_, stderr, err := r.runWrite(context.Background(), "commit", "--allow-empty", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit --allow-empty: %w: %s", err, stderr)
	}
	return nil
}

func CommitEmpty(message string) error { return CWD().CommitEmpty(message) }
