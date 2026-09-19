package completions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
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
			"clone", "switch", "branch", "checkout-pr", "add-remote", "completion", "status", "push",
			"pull", "tree", "diff", "doctor",
			"clean", "delete", "replay-list", "empty", "release", "worktree-switch",
			// clone flag
			"depth",
			// tree / diff flags
			"since", "mode",
			// clean flag names (bare so fish's `-l changes` matches too)
			"changes", "untracked", "ignored",
			// status flag names
			"flat", "follow",
			// status sections
			"worktree", "changes", "head",
			// release bumps and flag
			"patch", "minor", "major", "force",
			// completion shell choices and flag
			"bash", "fish", "zsh", "nu", "cd",
		},
		cases: []completionCase{
			{"top-level", []string{""}, []string{"clone", "switch", "branch", "clean", "release", "replay-list", "pull", "tree", "diff", "doctor", "worktree-switch"}},
			{"clone flags", []string{"clone", "--"}, []string{"--depth"}},
			{"tree flags", []string{"tree", "--"}, []string{"--since", "--all", "--follow"}},
			{"diff flags", []string{"diff", "--"}, []string{"--mode", "--follow"}},
			{"clean flags", []string{"clean", "--"}, []string{"--changes", "--untracked", "--ignored", "--all", "--yes"}},
			{"status flags", []string{"status", "--"}, []string{"--all", "--flat", "--follow"}},
			{"status sections", []string{"status", ""}, []string{"all", "branch", "remote", "worktree", "changes", "head"}},
			{"status alias sections", []string{"s", ""}, []string{"all", "branch", "remote", "worktree", "changes", "head"}},
			{"tree alias flags", []string{"t", "--"}, []string{"--since", "--all", "--follow"}},
			{"branch alias flags", []string{"b", "--"}, []string{"--help"}},
			{"push alias flags", []string{"p", "--"}, []string{"--help"}},
			{"worktree back flags", []string{"worktree-switch-", "--"}, []string{"--help"}},
			{"worktree back alias flags", []string{"w-", "--"}, []string{"--help"}},
			{"completion shells", []string{"completion", ""}, []string{"bash", "fish", "zsh", "nu"}},
			{"completion flags", []string{"completion", "--"}, []string{"--cd"}},
			{"release bumps", []string{"release", ""}, []string{"patch", "minor", "major"}},
			{"release flags", []string{"release", "--"}, []string{"--force"}},
		},
		nuHelpExpect: []string{"clone", "switch", "branch", "checkout-pr", "add-remote", "completion", "clean", "release", "pull", "tree", "diff", "doctor", "worktree-switch"},
	},
	{
		// The cd wrapper, rendered under the short name it is normally sourced
		// as. It completes nothing itself; syntax, content and nu's help are
		// what matter, and TestShellWrapperCd drives its behaviour.
		cmdName: "gg",
		render:  RenderCd,
		requiredContent: []string{
			"worktree-switch", "cd", "GITGUM_CD_WRAPPER",
		},
		nuHelpExpect: []string{"worktree-switch"},
	},
	{
		cmdName: "ff",
		render:  RenderFuzzyfinder,
		bashFn:  "_ff_completion",
		requiredContent: []string{
			// flag names (bare so fish's `-l multi` matches too)
			"multi", "query", "prompt", "header", "select-1",
			"fast", "print-query", "reverse", "height", "completion",
			// completion shell choices
			"bash", "fish", "zsh", "nu",
		},
		cases: []completionCase{
			{"flags", []string{"-"}, []string{"--multi", "--query", "--prompt", "--header", "--select-1", "--fast", "--print-query", "--reverse", "--height", "--completion"}},
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
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), cmdName+"."+shell)
	require.NoError(t, os.WriteFile(file, []byte(rendered), 0o644))
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
				require.NoError(t, err, "syntax check failed: output:\n%s", out)
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
				require.NoError(t, err)
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
				require.NoError(t, err, "bash completion: output:\n%s", out)
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
				require.NoError(t, err, "fish completion: output:\n%s", out)
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
			require.NoError(t, err, "nu help %s: output:\n%s", b.cmdName, out)
			for _, want := range b.nuHelpExpect {
				assert.ContainsString(t, string(out), want)
			}
		})
	}
}

// wrapperShim writes a fake gitgum binary into a dir and returns the dir. The
// shim answers worktree-switch with the path in $GG_TARGET (help flags print
// HELP instead), naming the direction it was asked for on stderr as the real
// one names the worktrees, and echoes anything else back, so the wrapper
// function each completion script defines can be driven without building the
// real binary.
func wrapperShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
    w|w+|w-|worktree-switch|worktree-switch+|worktree-switch-)
        for a in "$@"; do
            case "$a" in -h|--help) echo HELP; exit 0;; esac
        done
        [ -n "$GG_TARGET" ] || { echo "no target" >&2; exit 1; }
        case "$1" in *-) echo "announce back" >&2;; *) echo "announce next" >&2;; esac
        echo "$GG_TARGET"
        ;;
    *) echo "passthrough $* wrapper=$GITGUM_CD_WRAPPER" ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gitgum"), []byte(script), 0o755), "write shim")
	return dir
}

// TestShellWrapperCd: the gitgum function the cd script defines cds on
// `gitgum w`, leaves help output alone, stays put when the binary fails, and
// passes every other command through with the marker set. The completion
// script is sourced first, as a user would, so the two must coexist (nu in
// particular: the cd command shadows the completion extern).
func TestShellWrapperCd(t *testing.T) {
	shim := wrapperShim(t)
	target, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	start, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// Each script sources completions then the cd file, runs the wrapper,
	// prints pwd.
	scripts := map[string]func(files, cmd string) string{
		"bash": func(files, cmd string) string { return files + cmd + "\npwd" },
		"zsh":  func(files, cmd string) string { return "autoload -Uz compinit; compinit -C\n" + files + cmd + "\npwd" },
		"fish": func(files, cmd string) string { return files + cmd + "\npwd" },
		// nu aborts a script on a failed external, so the call is caught to let
		// the pwd print; a try block shares the caller's cwd.
		"nu": func(files, cmd string) string { return files + "try { " + cmd + " }\npwd" },
	}
	cases := []struct {
		name    string
		cmd     string
		env     string // GG_TARGET value; empty makes the shim fail
		wantOut string
		wantPwd string
	}{
		{"w cds", "gitgum w", target, "announce next", target},
		{"long form cds", "gitgum worktree-switch 2", target, "announce next", target},
		{"w+ cds", "gitgum w+", target, "announce next", target},
		{"long + form cds", "gitgum worktree-switch+ 2", target, "announce next", target},
		{"w- cds", "gitgum w-", target, "announce back", target},
		{"long - form cds", "gitgum worktree-switch-", target, "announce back", target},
		{"w- help stays put", "gitgum w- --help", target, "HELP", start},
		{"help stays put", "gitgum w --help", target, "HELP", start},
		{"failure stays put", "gitgum w", "", "", start},
		{"passthrough", "gitgum status --flat", target, "passthrough status --flat wrapper=1", start},
	}
	for _, s := range shells {
		path := lookupShellOnce(t, s.bin)
		files := "source " + renderToTemp(t, Render, "gitgum", s.template) + "\n" +
			"source " + renderToTemp(t, RenderCd, "gitgum", s.template) + "\n"
		for _, tc := range cases {
			t.Run(s.template+"/"+tc.name, func(t *testing.T) {
				if path == "" {
					t.Skipf("%s not installed", s.bin)
				}
				cmd := exec.Command(path, "-c", scripts[s.template](files, tc.cmd))
				cmd.Dir = start
				cmd.Env = append(os.Environ(),
					"PATH="+shim+string(os.PathListSeparator)+os.Getenv("PATH"),
					"GG_TARGET="+tc.env)
				out, _ := cmd.CombinedOutput()
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				gotPwd := lines[len(lines)-1]
				assert.Equal(t, gotPwd, tc.wantPwd, "pwd after %q; output:\n%s", tc.cmd, out)
				if tc.wantOut != "" {
					assert.ContainsString(t, string(out), tc.wantOut)
				}
			})
		}
	}
}
