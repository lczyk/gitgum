package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// minGitVersion is the lowest git release gg will run against.
//
// 2.35.2 is the floor because:
//   - safe.directory (2.35.2+) -- the read prelude relies on
//     -c safe.directory=<dir> to operate in repos owned by other UIDs
//     once we blank GIT_CONFIG_GLOBAL.
//   - GIT_CONFIG_GLOBAL (2.32+) and for-each-ref --format='%(upstream:short)'
//     (2.13+) sit below it.
//
// See meanderings/consistent-git-backend.prop.md.
var minGitVersion = [3]int{2, 35, 2}

// readPrelude is appended to argv before the user-supplied args for read
// invocations. -c flags lock parse-stable knobs even if a repo-local
// .git/config tries to override them; -C <dir> is added per-call from r.Dir.
var readPrelude = []string{
	"--no-pager",
	"-c", "color.ui=never",
	"-c", "core.quotepath=false",
	"-c", "status.relativePaths=true",
	"-c", "log.showSignature=false",
}

// writePrelude is the equivalent for write invocations. It deliberately
// omits status.relativePaths and log.showSignature -- those only affect
// reads -- but keeps the rest so writes don't emit ansi escapes or octal-
// escaped paths into our captured output.
var writePrelude = []string{
	"--no-pager",
	"-c", "color.ui=never",
	"-c", "core.quotepath=false",
}

// readEnv builds the env for read invocations. Strips:
//   - GIT_CONFIG_GLOBAL / GIT_CONFIG_SYSTEM -- ignore user/system gitconfig.
//     Repo-local .git/config is still honoured.
//   - GIT_DIR / GIT_WORK_TREE / GIT_INDEX_FILE / GIT_COMMON_DIR -- prevent a
//     parent shell pointing gg at a repo other than r.Dir.
//   - GIT_TRACE family -- user tracing pollutes parsed stderr.
//   - GIT_PAGER -- belt for --no-pager on old git.
//
// Forces:
//   - LC_ALL=C -- stable english error strings for any residual stderr
//     matching.
//   - GIT_TERMINAL_PROMPT=0 -- network reads fail fast instead of hanging
//     on credential prompts.
func readEnv() []string {
	return buildEnv(true)
}

// writeEnv builds the env for write invocations. Same as readEnv but
// keeps GIT_CONFIG_GLOBAL / GIT_CONFIG_SYSTEM so user identity (user.name,
// user.email), signing config, and hooks dir continue to work.
func writeEnv() []string {
	return buildEnv(false)
}

func buildEnv(strictConfig bool) []string {
	stripped := map[string]bool{
		"GIT_DIR":               true,
		"GIT_WORK_TREE":         true,
		"GIT_INDEX_FILE":        true,
		"GIT_COMMON_DIR":        true,
		"GIT_TRACE":             true,
		"GIT_TRACE_PACKET":      true,
		"GIT_TRACE_PACK_ACCESS": true,
		"GIT_TRACE_PERFORMANCE": true,
		"GIT_TRACE_SETUP":       true,
		"GIT_TRACE_CURL":        true,
		"GIT_TRACE2":            true,
		"GIT_TRACE2_EVENT":      true,
		"GIT_TRACE2_PERF":       true,
		"GIT_PAGER":             true,
	}
	if strictConfig {
		stripped["GIT_CONFIG_GLOBAL"] = true
		stripped["GIT_CONFIG_SYSTEM"] = true
	}
	out := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || stripped[key] {
			continue
		}
		out = append(out, kv)
	}
	if strictConfig {
		out = append(out,
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
	}
	out = append(out,
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
	)
	return out
}

// runRead executes a read-only git invocation. It returns raw (untrimmed)
// stdout and stderr; callers TrimSpace as needed (porcelain output is
// whitespace-sensitive).
func (r Repo) runRead(ctx context.Context, args ...string) (string, string, error) {
	if err := ensureMinVersion(ctx); err != nil {
		return "", "", err
	}
	full := buildArgs(r.Dir, readPrelude, args, true)
	return runCaptured(ctx, full, readEnv())
}

// runWrite executes a write git invocation. User identity, signing, and
// hooks are preserved.
func (r Repo) runWrite(ctx context.Context, args ...string) (string, string, error) {
	if err := ensureMinVersion(ctx); err != nil {
		return "", "", err
	}
	full := buildArgs(r.Dir, writePrelude, args, false)
	return runCaptured(ctx, full, writeEnv())
}

// runWriteStreaming executes a write that needs live stderr passthrough --
// fetch / push / clone progress that the user wants to see in real time,
// or hooks (pre-commit, post-checkout) whose output the user wants live.
// Bytes still flow to os.Stdout / os.Stderr unchanged; in addition, the
// last tailBufSize bytes of each stream are captured and returned so the
// caller can include them in wrapped error messages.
func (r Repo) runWriteStreaming(ctx context.Context, args ...string) (string, string, error) {
	if err := ensureMinVersion(ctx); err != nil {
		return "", "", err
	}
	full := buildArgs(r.Dir, writePrelude, args, false)
	return runStreaming(ctx, full, writeEnv())
}

// buildArgs prepends the prelude and (if dir is non-empty and absolute)
// -C <dir> + -c safe.directory=<dir>. safe.directory is added per-call
// rather than blanket * so we don't accidentally bypass git's ownership
// check for repos we weren't asked to touch.
func buildArgs(dir string, prelude, args []string, strictConfig bool) []string {
	full := make([]string, 0, len(prelude)+len(args)+4)
	full = append(full, prelude...)
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil && strictConfig {
			full = append(full, "-c", "safe.directory="+abs)
		}
		full = append(full, "-C", dir)
	}
	full = append(full, args...)
	return full
}

func runCaptured(ctx context.Context, args, env []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// tailBufSize bounds the per-stream capture for runStreaming. 8 KiB is
// enough to surface the last few lines of git output (typical error tail)
// without holding multi-megabyte fetch / push payloads in memory.
const tailBufSize = 8 * 1024

// tailBuffer is an io.Writer that retains only the last n bytes written.
// Used to capture a bounded tail of streamed git output for error context.
type tailBuffer struct {
	n   int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	if len(p) >= t.n {
		t.buf = append(t.buf[:0], p[len(p)-t.n:]...)
		return len(p), nil
	}
	if len(t.buf)+len(p) <= t.n {
		t.buf = append(t.buf, p...)
		return len(p), nil
	}
	drop := len(t.buf) + len(p) - t.n
	t.buf = append(t.buf[:0], t.buf[drop:]...)
	t.buf = append(t.buf, p...)
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.buf) }

func runStreaming(ctx context.Context, args, env []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	outTail := &tailBuffer{n: tailBufSize}
	errTail := &tailBuffer{n: tailBufSize}
	cmd.Stdout = io.MultiWriter(os.Stdout, outTail)
	cmd.Stderr = io.MultiWriter(os.Stderr, errTail)
	err := cmd.Run()
	return outTail.String(), errTail.String(), err
}

var (
	versionOnce sync.Once
	versionErr  error
)

// ensureMinVersion checks `git --version` once per process and caches the
// result. Below the floor, every subsequent helper call returns the same
// error so the user sees a clear message rather than cryptic fallout from
// missing config knobs.
func ensureMinVersion(ctx context.Context) error {
	versionOnce.Do(func() {
		versionErr = checkVersion(ctx)
	})
	return versionErr
}

func checkVersion(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "--version")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running git --version: %w", err)
	}
	v, err := parseGitVersion(string(out))
	if err != nil {
		return err
	}
	if compareVersion(v, minGitVersion) < 0 {
		return fmt.Errorf("git %d.%d.%d is below gg's minimum %d.%d.%d -- please upgrade",
			v[0], v[1], v[2],
			minGitVersion[0], minGitVersion[1], minGitVersion[2])
	}
	return nil
}

// parseGitVersion extracts the X.Y.Z triple from `git --version` output,
// e.g. "git version 2.43.0\n" -> [2, 43, 0]. Trailing build/commit suffixes
// (".windows.1", ".gk.something") are tolerated; missing patch defaults to 0.
func parseGitVersion(s string) ([3]int, error) {
	var v [3]int
	s = strings.TrimSpace(s)
	const prefix = "git version "
	if !strings.HasPrefix(s, prefix) {
		return v, fmt.Errorf("unexpected git --version output: %q", s)
	}
	rest := s[len(prefix):]
	if i := strings.IndexAny(rest, " \t\r\n"); i >= 0 {
		rest = rest[:i]
	}
	parts := strings.SplitN(rest, ".", 4)
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return v, fmt.Errorf("parsing git version %q: %w", s, err)
		}
		v[i] = n
	}
	return v, nil
}

func compareVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// resetVersionCheck is exposed for tests; not part of the public API.
func resetVersionCheck() {
	versionOnce = sync.Once{}
	versionErr = nil
}
