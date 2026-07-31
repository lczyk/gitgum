package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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
// when the key is unset. Reads honour repo-local .git/config.
//
// Exit 1 is the only code meaning "no such key"; git reserves the rest for real
// faults (2 invalid section/key, 3 invalid config file, 128 not a repo). Those
// propagate rather than reading as absence, so a broken config surfaces instead
// of quietly making every branch look unconfigured.
func (r Repo) BranchConfigGet(branch, key string) (string, error) {
	name := fmt.Sprintf("branch.%s.%s", branch, key)
	stdout, stderr, err := r.run("config", "--get", name)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil // unset
		}
		return "", fmt.Errorf("git config --get %s: %w: %s", name, err, stderr)
	}
	return stdout, nil
}
