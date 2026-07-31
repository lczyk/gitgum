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

// TagExists reports whether a tag of the given name exists. False on any error
// (treats unresolvable refs as absent).
//
// The ref is spelled refs/tags/<name> rather than bare <name>: rev-parse walks
// git's whole ref precedence for an unqualified name, so a *branch* called
// v1.2.3 would otherwise read as a tag and make `gg release` refuse to tag.
func (r Repo) TagExists(name string) bool {
	_, _, err := r.run("rev-parse", "--verify", "--quiet", "refs/tags/"+name)
	return err == nil
}
