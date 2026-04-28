// cspell:ignore CWORD COMPREPLY

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

// binSpec describes one binary's completion surface — render function, the
// content tokens we expect to see in any rendered template, and the bash
// completion entry-point function name (for direct-drive bash tests).
type binSpec struct {
	cmdName         string
	render          func(shell, cmd string) (string, error)
	bashFn          string
	requiredContent []string
	cases           []completionCase
	nuHelpExpect    []string
}

// completionCase drives a shell's completer: words is the token sequence
// after the command name (last token = partial being completed). Output must
// contain every wantContains string.
type completionCase struct {
	name         string
	words        []string
	wantContains []string
}

var bins = []binSpec{
	{
		cmdName: "gitgum",
		render:  Render,
		bashFn:  "_gitgum_completion",
		requiredContent: []string{
			// subcommands
			"switch", "checkout-pr", "completion", "status", "push",
			"clean", "delete", "replay-list", "empty", "release",
			// clean flag names (bare so fish's `-l changes` matches too)
			"changes", "untracked", "ignored",
			// release bumps
			"patch", "minor", "major",
			// completion shell choices
			"bash", "fish", "zsh", "nu",
		},
		cases: []completionCase{
			{"top-level", []string{""}, []string{"switch", "clean", "release", "replay-list"}},
			{"clean flags", []string{"clean", "--"}, []string{"--changes", "--untracked", "--ignored", "--all", "--yes"}},
			{"completion shells", []string{"completion", ""}, []string{"bash", "fish", "zsh", "nu"}},
			{"release bumps", []string{"release", ""}, []string{"patch", "minor", "major"}},
		},
		nuHelpExpect: []string{"switch", "checkout-pr", "completion", "clean", "release"},
	},
	{
		cmdName: "ff",
		render:  RenderFuzzyfinder,
		bashFn:  "_ff_completion",
		requiredContent: []string{
			// flag names (bare so fish's `-l multi` matches too)
			"multi", "query", "prompt", "header", "select-1",
			"fast", "reverse", "height", "completion",
			// completion shell choices
			"bash", "fish", "zsh", "nu",
		},
		cases: []completionCase{
			{"flags", []string{"-"}, []string{"--multi", "--query", "--prompt", "--header", "--select-1", "--fast", "--reverse", "--height", "--completion"}},
			{"completion shells", []string{"--completion", ""}, []string{"bash", "fish", "zsh", "nu"}},
		},
		nuHelpExpect: []string{"multi", "query", "completion"},
	},
}

func lookupShell(t *testing.T, bin string) string {
	t.Helper()
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not installed", bin)
	}
	versionOut, _ := exec.Command(path, "--version").CombinedOutput()
	version := strings.TrimSpace(strings.SplitN(string(versionOut), "\n", 2)[0])
	t.Logf("%s: %s", path, version)
	return path
}

func renderToTemp(t *testing.T, render func(shell, cmd string) (string, error), cmdName, shell string) string {
	t.Helper()
	rendered, err := render(shell, cmdName)
	assert.NoError(t, err)
	file := filepath.Join(t.TempDir(), cmdName+"."+shell)
	assert.NoError(t, os.WriteFile(file, []byte(rendered), 0o644))
	return file
}

// TestShellSyntax: every (binary, shell) rendered template parses cleanly.
// Skips shells that aren't installed.
func TestShellSyntax(t *testing.T) {
	for _, s := range shells {
		path := lookupShellOnce(t, s.bin)
		for _, b := range bins {
			t.Run(b.cmdName+"/"+s.template, func(t *testing.T) {
				if path == "" {
					t.Skipf("%s not installed", s.bin)
				}
				file := renderToTemp(t, b.render, b.cmdName, s.template)
				out, err := exec.Command(path, s.checkArgs(file)...).CombinedOutput()
				assert.NoError(t, err, "syntax check failed: output:\n%s", out)
			})
		}
	}
}

// lookupShellOnce: like lookupShell but returns "" instead of skipping when
// not found, so the caller can iterate inner subtests and skip each.
func lookupShellOnce(t *testing.T, bin string) string {
	t.Helper()
	path, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	versionOut, _ := exec.Command(path, "--version").CombinedOutput()
	version := strings.TrimSpace(strings.SplitN(string(versionOut), "\n", 2)[0])
	t.Logf("%s: %s", path, version)
	return path
}

// TestShellContent: every rendered template contains the expected tokens for
// its binary. No shell binary needed.
func TestShellContent(t *testing.T) {
	for _, b := range bins {
		for _, s := range shells {
			t.Run(b.cmdName+"/"+s.template, func(t *testing.T) {
				rendered, err := b.render(s.template, b.cmdName)
				assert.NoError(t, err)
				assert.That(t, !strings.Contains(rendered, Placeholder), "placeholder leftover")
				assert.ContainsString(t, rendered, b.cmdName)
				for _, want := range b.requiredContent {
					assert.ContainsString(t, rendered, want)
				}
			})
		}
	}
}

func bashQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// TestBashCompletion: drives each binary's bash completion function with
// synthesized COMP_WORDS / COMP_CWORD per scenario, asserts COMPREPLY.
func TestBashCompletion(t *testing.T) {
	bash := lookupShell(t, "bash")
	for _, b := range bins {
		file := renderToTemp(t, b.render, b.cmdName, "bash")
		for _, tc := range b.cases {
			t.Run(b.cmdName+"/"+tc.name, func(t *testing.T) {
				full := append([]string{b.cmdName}, tc.words...)
				cwIdx := len(full) - 1
				cur := full[cwIdx]
				prev := full[cwIdx-1]

				var arr strings.Builder
				for _, w := range full {
					arr.WriteString(bashQuote(w))
					arr.WriteByte(' ')
				}
				script := fmt.Sprintf(
					"source %s\nCOMP_WORDS=(%s)\nCOMP_CWORD=%d\n%s %s %s %s\nprintf '%%s\\n' \"${COMPREPLY[@]}\"\n",
					bashQuote(file), arr.String(), cwIdx, b.bashFn,
					bashQuote(b.cmdName), bashQuote(cur), bashQuote(prev),
				)
				out, err := exec.Command(bash, "-c", script).CombinedOutput()
				assert.NoError(t, err, "bash completion: output:\n%s", out)
				for _, want := range tc.wantContains {
					assert.ContainsString(t, string(out), want)
				}
			})
		}
	}
}

// TestFishCompletion: uses fish's `complete -C` per scenario.
func TestFishCompletion(t *testing.T) {
	fish := lookupShell(t, "fish")
	for _, b := range bins {
		file := renderToTemp(t, b.render, b.cmdName, "fish")
		for _, tc := range b.cases {
			t.Run(b.cmdName+"/"+tc.name, func(t *testing.T) {
				cmdline := b.cmdName + " " + strings.Join(tc.words, " ")
				script := fmt.Sprintf("source %q; complete -C %q", file, cmdline)
				out, err := exec.Command(fish, "-c", script).CombinedOutput()
				assert.NoError(t, err, "fish completion: output:\n%s", out)
				for _, want := range tc.wantContains {
					assert.ContainsString(t, string(out), want)
				}
			})
		}
	}
}

// TestNuCompletion: structural check via `help <cmdname>`. Nu's interactive
// completion engine isn't directly invocable from a script.
func TestNuCompletion(t *testing.T) {
	nu := lookupShell(t, "nu")
	for _, b := range bins {
		t.Run(b.cmdName, func(t *testing.T) {
			file := renderToTemp(t, b.render, b.cmdName, "nu")
			out, err := exec.Command(nu, "-c", "source "+file+"; help "+b.cmdName).CombinedOutput()
			assert.NoError(t, err, "nu help %s: output:\n%s", b.cmdName, out)
			for _, want := range b.nuHelpExpect {
				assert.ContainsString(t, string(out), want)
			}
		})
	}
}
