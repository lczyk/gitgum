package completions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
)

// shellSpec describes how to invoke a shell to run a syntax check on a
// rendered completion script.
type shellSpec struct {
	template  string // Render() shell key
	bin       string // executable name
	checkArgs func(file string) []string
}

var shells = []shellSpec{
	{template: "bash", bin: "bash", checkArgs: func(f string) []string { return []string{"-n", f} }},
	{template: "zsh", bin: "zsh", checkArgs: func(f string) []string { return []string{"-n", f} }},
	{template: "fish", bin: "fish", checkArgs: func(f string) []string { return []string{"-n", f} }},
	{template: "nu", bin: "nu", checkArgs: func(f string) []string { return []string{"-c", "source " + f} }},
}

func lookupShell(t *testing.T, bin string) string {
	t.Helper()
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not installed", bin)
	}
	vout, _ := exec.Command(path, "--version").CombinedOutput()
	version := strings.TrimSpace(strings.SplitN(string(vout), "\n", 2)[0])
	t.Logf("%s: %s", path, version)
	return path
}

func renderToTemp(t *testing.T, template, cmdName string) string {
	t.Helper()
	rendered, err := Render(template, cmdName)
	assert.NoError(t, err)
	file := filepath.Join(t.TempDir(), cmdName+"."+template)
	assert.NoError(t, os.WriteFile(file, []byte(rendered), 0o644))
	return file
}

// TestShellSyntax: rendered template parses cleanly under each shell's
// parse-only mode. Skips shells that aren't installed.
func TestShellSyntax(t *testing.T) {
	for _, s := range shells {
		t.Run(s.template, func(t *testing.T) {
			path := lookupShell(t, s.bin)
			file := renderToTemp(t, s.template, "gitgum")
			out, err := exec.Command(path, s.checkArgs(file)...).CombinedOutput()
			assert.NoError(t, err, "syntax check failed: output:\n%s", out)
		})
	}
}

// requiredContent: bare tokens every rendered template must contain. Bare
// (no `--` prefix) so the same list works for fish (`-l changes`) and the
// others (`--changes`). Behavior tests cover exact flag wiring.
var requiredContent = []string{
	// subcommands
	"switch", "checkout-pr", "completion", "status", "push",
	"clean", "delete", "replay-list", "empty", "release",
	// clean flag names
	"changes", "untracked", "ignored",
	// release bumps
	"patch", "minor", "major",
	// completion shells
	"bash", "fish", "zsh", "nu",
}

// TestShellContent: every shell template contains the full set of
// subcommands, flags, and choice values; placeholder is fully substituted.
func TestShellContent(t *testing.T) {
	for _, s := range shells {
		t.Run(s.template, func(t *testing.T) {
			rendered, err := Render(s.template, "gitgum")
			assert.NoError(t, err)
			assert.That(t, !strings.Contains(rendered, Placeholder), "placeholder leftover")
			assert.ContainsString(t, rendered, "gitgum")
			for _, want := range requiredContent {
				assert.ContainsString(t, rendered, want)
			}
		})
	}
}

// completionCase drives a shell's completer: words is the token sequence
// after the command name (last token = partial being completed). Output must
// contain every wantContains string.
type completionCase struct {
	name         string
	words        []string
	wantContains []string
}

var completionCases = []completionCase{
	{"top-level", []string{""}, []string{"switch", "clean", "release", "replay-list"}},
	{"clean flags", []string{"clean", "--"}, []string{"--changes", "--untracked", "--ignored", "--all", "--yes"}},
	{"completion shells", []string{"completion", ""}, []string{"bash", "fish", "zsh", "nu"}},
	{"release bumps", []string{"release", ""}, []string{"patch", "minor", "major"}},
}

func bashQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// TestBashCompletion: drives _gitgum_completion with synthesized COMP_WORDS /
// COMP_CWORD for each scenario and checks COMPREPLY contents.
func TestBashCompletion(t *testing.T) {
	bash := lookupShell(t, "bash")
	file := renderToTemp(t, "bash", "gitgum")
	for _, tc := range completionCases {
		t.Run(tc.name, func(t *testing.T) {
			full := append([]string{"gitgum"}, tc.words...)
			cwIdx := len(full) - 1
			cur := full[cwIdx]
			prev := full[cwIdx-1]

			var arr strings.Builder
			for _, w := range full {
				arr.WriteString(bashQuote(w))
				arr.WriteByte(' ')
			}
			script := fmt.Sprintf(
				"source %s\nCOMP_WORDS=(%s)\nCOMP_CWORD=%d\n_gitgum_completion gitgum %s %s\nprintf '%%s\\n' \"${COMPREPLY[@]}\"\n",
				bashQuote(file), arr.String(), cwIdx, bashQuote(cur), bashQuote(prev),
			)
			out, err := exec.Command(bash, "-c", script).CombinedOutput()
			assert.NoError(t, err, "bash completion: output:\n%s", out)
			for _, want := range tc.wantContains {
				assert.ContainsString(t, string(out), want)
			}
		})
	}
}

// TestFishCompletion: uses fish's `complete -C` to ask the rendered script
// what to suggest for each scenario's command line.
func TestFishCompletion(t *testing.T) {
	fish := lookupShell(t, "fish")
	file := renderToTemp(t, "fish", "gitgum")
	for _, tc := range completionCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdline := "gitgum " + strings.Join(tc.words, " ")
			script := fmt.Sprintf("source %q; complete -C %q", file, cmdline)
			out, err := exec.Command(fish, "-c", script).CombinedOutput()
			assert.NoError(t, err, "fish completion: output:\n%s", out)
			for _, want := range tc.wantContains {
				assert.ContainsString(t, string(out), want)
			}
		})
	}
}

// TestNuCompletion: verifies externs registered with expected subcommands by
// inspecting `help` output. Nu's interactive completion engine isn't directly
// invocable from a script, so this is a structural check.
func TestNuCompletion(t *testing.T) {
	nu := lookupShell(t, "nu")
	file := renderToTemp(t, "nu", "gitgum")
	out, err := exec.Command(nu, "-c", "source "+file+"; help gitgum").CombinedOutput()
	assert.NoError(t, err, "nu help gitgum: output:\n%s", out)
	for _, want := range []string{"switch", "checkout-pr", "completion", "clean", "release"} {
		assert.ContainsString(t, string(out), want)
	}
}
