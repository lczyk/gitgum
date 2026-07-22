package git

import (
	"context"
	"fmt"
)

// BranchConfigSet writes branch.<branch>.<key> to the repo-local config. Used
// to stash gg-specific per-branch metadata (e.g. which PR a branch mirrors)
// that git has no native home for.
func (r Repo) BranchConfigSet(branch, key, value string) error {
	name := fmt.Sprintf("branch.%s.%s", branch, key)
	if _, stderr, err := r.runWrite(context.Background(), "config", name, value); err != nil {
		return fmt.Errorf("git config %s: %w: %s", name, err, stderr)
	}
	return nil
}

// BranchConfigGet reads branch.<branch>.<key> from config, returning ("", nil)
// when the key is unset (git exits non-zero for a missing key -- that's not an
// error here, just absence). Reads honour repo-local .git/config.
func (r Repo) BranchConfigGet(branch, key string) (string, error) {
	name := fmt.Sprintf("branch.%s.%s", branch, key)
	stdout, _, err := r.run("config", "--get", name)
	if err != nil {
		// Unset key -> exit 1 with empty output; distinguish only by output.
		return "", nil
	}
	return stdout, nil
}

func BranchConfigSet(branch, key, value string) error {
	return CWD().BranchConfigSet(branch, key, value)
}

func BranchConfigGet(branch, key string) (string, error) {
	return CWD().BranchConfigGet(branch, key)
}
