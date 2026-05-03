package git

import (
	"context"
	"fmt"
)

// Add stages the given paths. Empty list errors (matches `git add` with no
// pathspec).
func (r Repo) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, stderr, err := r.runWrite(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("git add: %w: %s", err, stderr)
	}
	return nil
}

func Add(paths ...string) error { return CWD().Add(paths...) }
