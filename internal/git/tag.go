package git

import (
	"context"
	"fmt"
	"strings"
)

// TagAnnotated creates an annotated tag with the given message. Output is
// captured; on error, stderr is included in the wrapped message.
func (r Repo) TagAnnotated(name, message string) error {
	if _, stderr, err := r.runWrite(context.Background(), "tag", "-a", name, "-m", message); err != nil {
		return fmt.Errorf("git tag %s: %w: %s", name, err, strings.TrimSpace(stderr))
	}
	return nil
}

// TagExists reports whether a ref of the given name resolves. False on any
// error (treats unresolvable refs as absent).
func (r Repo) TagExists(name string) bool {
	_, _, err := r.run("rev-parse", "--verify", name)
	return err == nil
}

func TagAnnotated(name, message string) error { return CWD().TagAnnotated(name, message) }
func TagExists(name string) bool              { return CWD().TagExists(name) }
