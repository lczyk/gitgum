package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lczyk/assert"
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
		assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(stdout), "true")
}

// Poisoning user gitconfig must not leak into reads. Sets a global config
// pointing at a temp file with color.ui=always; expects no ansi escapes
// in captured output.
func TestRunReadIgnoresUserGlobalConfig(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	cfgDir := t.TempDir()
	cfg := filepath.Join(cfgDir, "config")
	assert.NoError(t, os.WriteFile(cfg, []byte("[color]\n\tui = always\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	r := Repo{Dir: dir}
	stdout, _, err := r.runRead(context.Background(), "status", "--short")
	assert.NoError(t, err)
	assert.That(t, !strings.Contains(stdout, "\x1b["), "stdout should not contain ansi escapes when read profile is active")
}

// GIT_DIR in the env must not redirect reads away from r.Dir.
func TestRunReadIgnoresGitDirEnv(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	other := temp_repo.NewRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	r := Repo{Dir: dir}
	stdout, _, err := r.runRead(context.Background(), "rev-parse", "--show-toplevel")
	assert.NoError(t, err)
	got, err := filepath.EvalSymlinks(strings.TrimSpace(stdout))
	assert.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	assert.NoError(t, err)
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
