package git

import (
	"context"
	"fmt"
)

// Checkout switches the working tree to the given branch (quiet mode).
// Caller should provide the user-facing context; the returned error wraps
// stderr so it surfaces in the message chain.
func (r Repo) Checkout(branch string) error {
	_, stderr, err := r.runWrite(context.Background(), "checkout", "--quiet", branch)
	if err != nil {
		return fmt.Errorf("git checkout %s: %w: %s", branch, err, stderr)
	}
	return nil
}

func Checkout(branch string) error { return CWD().Checkout(branch) }
