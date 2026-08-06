package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert/require"
)

// gitShim puts a fake `git` in front of the real one on PATH and returns a
// reader for the argv of every call made through it. PATH is the seam because
// internal/git execs "git" by name and strips the GIT_TRACE family from the
// child env, so git's own tracing cannot be used to count invocations.
//
// Callers must not t.Parallel: t.Setenv forbids it.
func gitShim(t *testing.T) (calls func() []string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err, "git must be on PATH")

	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>" + log + "\nexec " + realGit + " \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755), "write git shim")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return func() []string {
		raw, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(raw)), "\n")
	}
}

// gitArgs strips gg's read prelude from a logged argv, leaving the subcommand
// and its own arguments. -c and -C each take a value; the rest of the prelude
// is bare flags. A call that is all flags (`git --version`) yields nothing.
func gitArgs(argv string) []string {
	fields := strings.Fields(argv)
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if !strings.HasPrefix(f, "-") {
			return fields[i:]
		}
		if f == "-c" || f == "-C" {
			i++
		}
	}
	return nil
}

// countCalls counts invocations of one git subcommand.
func countCalls(calls []string, subcommand string) int {
	n := 0
	for _, argv := range calls {
		if args := gitArgs(argv); len(args) > 0 && args[0] == subcommand {
			n++
		}
	}
	return n
}

// countExact counts invocations whose subcommand and arguments match want
// exactly, for when the subcommand alone is too coarse (`git remote` the
// listing vs `git remote get-url` the lookup).
func countExact(calls []string, want ...string) int {
	n := 0
	for _, argv := range calls {
		if slices.Equal(gitArgs(argv), want) {
			n++
		}
	}
	return n
}
