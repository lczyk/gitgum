package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestParseGitVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    [3]int
		wantErr bool
	}{
		{"git version 2.43.0\n", [3]int{2, 43, 0}, false},
		{"git version 2.35.2", [3]int{2, 35, 2}, false},
		{"git version 2.42.0.windows.1\n", [3]int{2, 42, 0}, false},
		{"git version 2.13", [3]int{2, 13, 0}, false},
		{"git version 2.46.0 (Apple Git-153)", [3]int{2, 46, 0}, false},
		{"not git version 1.2.3", [3]int{}, true},
		{"git version not.a.number", [3]int{}, true},
	}
	for _, tc := range cases {
		got, err := parseGitVersion(tc.in)
		if tc.wantErr {
			assert.That(t, err != nil, "want error for ", tc.in)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, got, tc.want)
	}
}

func TestCompareVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b [3]int
		want int
	}{
		{[3]int{2, 35, 2}, [3]int{2, 35, 2}, 0},
		{[3]int{2, 35, 1}, [3]int{2, 35, 2}, -1},
		{[3]int{2, 36, 0}, [3]int{2, 35, 2}, 1},
		{[3]int{3, 0, 0}, [3]int{2, 99, 99}, 1},
		{[3]int{1, 99, 99}, [3]int{2, 0, 0}, -1},
	}
	for _, tc := range cases {
		assert.Equal(t, compareVersion(tc.a, tc.b), tc.want)
	}
}

func TestRunReadCapturesOutput(t *testing.T) {
	t.Parallel()
	r := Repo{Dir: temp_repo.NewRepo(t)}
	stdout, _, err := r.runRead(context.Background(), "rev-parse", "--is-inside-work-tree")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(stdout), "true")
}

// Poisoning user gitconfig must not leak into reads. Sets a global config
// pointing at a temp file with color.ui=always; expects no ansi escapes
// in captured output.
func TestRunReadIgnoresUserGlobalConfig(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	cfgDir := t.TempDir()
	cfg := filepath.Join(cfgDir, "config")
	require.NoError(t, os.WriteFile(cfg, []byte("[color]\n\tui = always\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	r := Repo{Dir: dir}
	stdout, _, err := r.runRead(context.Background(), "status", "--short")
	require.NoError(t, err)
	assert.That(t, !strings.Contains(stdout, "\x1b["), "stdout should not contain ansi escapes when read profile is active")
}

// Network reads go through runReadNet, which -- unlike runRead -- must keep
// the user's global gitconfig so private-repo auth (credential helpers,
// url.*.insteadOf, http.*.extraHeader) survives. Probe with a custom config
// key the read prelude does not lock: runReadNet sees it, plain runRead does
// not (it blanks GIT_CONFIG_GLOBAL to /dev/null).
func TestRunReadNetHonoursUserGlobalConfig(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	cfgDir := t.TempDir()
	cfg := filepath.Join(cfgDir, "config")
	require.NoError(t, os.WriteFile(cfg, []byte("[gitgum]\n\tmarker = netauth\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	r := Repo{Dir: dir}

	got, _, err := r.runReadNet(context.Background(), "config", "--get", "gitgum.marker")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(got), "netauth")

	// The same key is invisible to plain runRead, which blanks global config.
	blanked, _, _ := r.runRead(context.Background(), "config", "--get", "gitgum.marker")
	assert.Equal(t, strings.TrimSpace(blanked), "")
}

// GIT_DIR in the env must not redirect reads away from r.Dir.
func TestRunReadIgnoresGitDirEnv(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	other := temp_repo.NewRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	r := Repo{Dir: dir}
	stdout, _, err := r.runRead(context.Background(), "rev-parse", "--show-toplevel")
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(strings.TrimSpace(stdout))
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, got, want)
}

// Locale poisoning must not change error string shape -- LC_ALL=C forces
// english regardless of LANG.
func TestRunReadForcesCLocale(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	t.Setenv("LANG", "de_DE.UTF-8")

	r := Repo{Dir: dir}
	_, stderr, err := r.runRead(context.Background(), "rev-parse", "--abbrev-ref", "nonexistent-branch@{u}")
	assert.That(t, err != nil, "want error for nonexistent upstream")
	// stderr should be english "no upstream" or similar; assert it's ascii
	// at minimum (no german umlauts that LANG=de would produce).
	for _, b := range []byte(stderr) {
		assert.That(t, b < 0x80, "stderr byte should be ascii under LC_ALL=C, got ", int(b))
	}
}

// Ctx cancel propagates SIGKILL to the child.
func TestRunReadContextCancel(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	r := Repo{Dir: dir}
	// `gc` is one of the longer-running ops; even on a tiny repo the
	// cancel races the syscall. We don't care which path wins -- only
	// that we get back promptly without the test hanging.
	done := make(chan error, 1)
	go func() {
		_, _, err := r.runRead(ctx, "gc")
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runRead did not return after ctx cancel")
	}
}

// A read must not take index.lock: `gg status` run beside a build would
// otherwise fail on a lock the user did not create and cannot act on. Writes
// keep it -- they are there to change something.
func TestPreludesAndOptionalLocks(t *testing.T) {
	t.Parallel()
	const flag = "--no-optional-locks"
	assert.That(t, slices.Contains(buildArgs("", readPrelude, []string{"status"}, true), flag),
		"read prelude should carry %s", flag)
	assert.That(t, !slices.Contains(buildArgs("", writePrelude, []string{"commit"}, false), flag),
		"write prelude should not carry %s", flag)
}

// The read prelude locks the knobs that decide output *shape*. The diff
// algorithm is not one of them -- it decides the answer -- so it is not there.
func TestReadPreludeDoesNotPinTheDiffAlgorithm(t *testing.T) {
	t.Parallel()
	for _, arg := range buildArgs("", readPrelude, []string{"diff"}, true) {
		assert.That(t, !strings.HasPrefix(arg, "diff.algorithm="),
			"the algorithm is forwarded per repo, not locked in the prelude; got %q", arg)
	}
}

// A block replaced by a longer one, sharing a blank line and a closing brace
// with what replaced it. histogram takes the old block out whole and counts
// 20/9; myers, minimal and patience all stitch the shared lines into the new
// block and count 19/8. So the numbers below say which algorithm ran.
const (
	beforeReplacedBlock = `#ifndef GUARD_ONE_H
#define GUARD_ONE_H

#include <types.h>

struct thing_info {
	char	name[16];
	int	interval;

	/* internal */
	struct thing	*est;
};

#endif /* GUARD_ONE_H */
`
	afterReplacedBlock = `#ifndef GUARD_TWO_H
#define GUARD_TWO_H

#include <types.h>

enum thing_flags {
	THING_INVERT	= 1<<0,
	THING_ABS	= 1<<1,
	THING_REL	= 1<<2,
};

enum thing_mode {
	THING_NONE,
	THING_EQ,
	THING_LT,
};

struct thing_match {
	char	name1[16];
	char	name2[16];
	int	flags;
	int	mode;
};

#endif /* GUARD_TWO_H */
`
)

// replacedBlockRepo stages the fixture above and returns the repo dir.
func replacedBlockRepo(t *testing.T) string {
	t.Helper()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "f.h", beforeReplacedBlock, "chore: base")
	temp_repo.WriteFile(t, dir, "f.h", afterReplacedBlock)
	return dir
}

func countedLines(t *testing.T, dir string) (added, deleted int) {
	t.Helper()
	forwardedConfigCache.Delete(dir)
	files, err := Repo{Dir: dir}.DiffFiles()
	require.NoError(t, err, "reading the diff")
	require.Equal(t, len(files), 1, "one changed file")
	return files[0].Added, files[0].Deleted
}

// A repo-local diff.algorithm reaches the read: gg reporting a different
// number of insertions to the git sitting next to it is the surprise the
// forwarding exists to avoid.
func TestDiffFilesHonoursRepoDiffAlgorithm(t *testing.T) {
	t.Parallel()
	dir := replacedBlockRepo(t)

	temp_repo.RunGit(t, dir, "config", "diff.algorithm", "histogram")
	added, deleted := countedLines(t, dir)
	assert.Equal(t, added, 20)
	assert.Equal(t, deleted, 9)

	for _, algo := range []string{"myers", "minimal", "patience"} {
		temp_repo.RunGit(t, dir, "config", "diff.algorithm", algo)
		added, deleted := countedLines(t, dir)
		assert.Equal(t, added, 19, "diff.algorithm=%q should reach the read", algo)
		assert.Equal(t, deleted, 8, "diff.algorithm=%q should reach the read", algo)
	}
}

// The setting usually lives in the user's global config, which the read env
// blanks -- so forwarding it is the whole point rather than an edge case.
func TestDiffFilesHonoursGlobalDiffAlgorithm(t *testing.T) {
	dir := replacedBlockRepo(t)
	cfg := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(cfg, []byte("[diff]\n\talgorithm = histogram\n"), 0o644), "write global config")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	added, deleted := countedLines(t, dir)
	assert.Equal(t, added, 20)
	assert.Equal(t, deleted, 9)
}

// Nothing configured anywhere means git's own default, which is what the
// user's git would do too. The global config is blanked because the machine
// running the tests may well have chosen one.
func TestDiffFilesWithoutConfiguredAlgorithm(t *testing.T) {
	dir := replacedBlockRepo(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty"))

	added, deleted := countedLines(t, dir)
	assert.Equal(t, added, 19)
	assert.Equal(t, deleted, 8)
}

// An algorithm git does not know is forwarded anyway and refused, which is
// what the user's own git does with the same config. Substituting a working
// default here would have gg disagree with the git that just failed.
func TestDiffFilesForwardsUnknownAlgorithm(t *testing.T) {
	t.Parallel()
	dir := replacedBlockRepo(t)
	temp_repo.RunGit(t, dir, "config", "diff.algorithm", "nonsense")
	forwardedConfigCache.Delete(dir)

	_, err := Repo{Dir: dir}.DiffFiles()
	assert.Error(t, err, assert.AnyError, "git refuses an algorithm it does not know")
}

// diff.renames decides whether a file that moved is one row naming both ends
// or two rows naming one end each -- the answer, not its shape, so gg forwards
// it rather than choosing for the user.
func TestDiffFilesHonoursDiffRenames(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "old.txt", "one\ntwo\nthree\nfour\n", "chore: base")
	temp_repo.RunGit(t, dir, "mv", "old.txt", "new.txt")

	temp_repo.RunGit(t, dir, "config", "diff.renames", "true")
	forwardedConfigCache.Delete(dir)
	files, err := Repo{Dir: dir}.DiffFiles("--cached")
	require.NoError(t, err, "reading the diff")
	require.Equal(t, len(files), 1, "a detected rename is one row")
	assert.Equal(t, files[0].RenamedFrom, "old.txt")
	assert.Equal(t, files[0].Path, "new.txt")

	temp_repo.RunGit(t, dir, "config", "diff.renames", "false")
	forwardedConfigCache.Delete(dir)
	files, err = Repo{Dir: dir}.DiffFiles("--cached")
	require.NoError(t, err, "reading the diff")
	require.Equal(t, len(files), 2, "without detection the move is a delete and an add")
	for _, f := range files {
		assert.Equal(t, f.RenamedFrom, "")
	}
}

// The setting usually lives in the user's global config, which the read env
// blanks -- so forwarding it is the whole point rather than an edge case.
func TestDiffFilesHonoursGlobalDiffRenames(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "old.txt", "one\ntwo\nthree\nfour\n", "chore: base")
	temp_repo.RunGit(t, dir, "mv", "old.txt", "new.txt")

	cfg := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(cfg, []byte("[diff]\n\trenames = false\n"), 0o644), "write global config")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	forwardedConfigCache.Delete(dir)

	files, err := Repo{Dir: dir}.DiffFiles("--cached")
	require.NoError(t, err, "reading the diff")
	assert.Equal(t, len(files), 2)
}

// Only the named keys travel: the rest of the diff section is the user's
// business with their own git, not something gg should reproduce.
//
// The global config is blanked because the machine running the tests may well
// have set one of the named keys, which would look like a leak from this repo.
func TestForwardedConfigCarriesOnlyTheNamedKeys(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty"))
	temp_repo.RunGit(t, dir, "config", "diff.algorithm", "patience")
	temp_repo.RunGit(t, dir, "config", "diff.tool", "meld")
	temp_repo.RunGit(t, dir, "config", "diff.context", "7")
	forwardedConfigCache.Delete(dir)

	args := Repo{Dir: dir}.forwardedConfigArgs(context.Background())
	assert.EqualArrays(t, args, []string{"-c", "diff.algorithm=patience"})
}
