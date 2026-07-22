package commands

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
)

// mustRef parses raw or fails the test; keeps the table cases terse.
func mustRef(t *testing.T, raw string) git.RepoRef {
	t.Helper()
	ref, ok := git.ParseRepoRef(raw)
	require.That(t, ok, "ParseRepoRef(%q)", raw)
	return ref
}

func TestBuildClonePlan(t *testing.T) {
	t.Parallel() // stdout isn't a tty under `go test`, so notes carry no ansi.

	tests := []struct {
		name       string
		raw        string // the spelling the user typed
		dir        string
		depth      int
		wantArgs   []string
		wantRemote string
		wantDir    string
		wantNote   string // substring; "" means no note expected
	}{
		{
			name:       "host shorthand reconstructs canonical https + names remote",
			raw:        "github.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "www is normalised away",
			raw:        "www.github.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "full https url is cloned verbatim",
			raw:        "https://gitlab.com/canonical/cbs-tools",
			wantArgs:   []string{"clone", "-o", "canonical", "https://gitlab.com/canonical/cbs-tools", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "ssh url is not downgraded to https",
			raw:        "git@github.com:canonical/cbs-tools.git",
			wantArgs:   []string{"clone", "-o", "canonical", "git@github.com:canonical/cbs-tools.git", "cbs-tools"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools",
		},
		{
			name:       "codeberg + depth",
			raw:        "codeberg.org/lczyk/gitgum",
			depth:      1,
			wantArgs:   []string{"clone", "-o", "lczyk", "--depth", "1", "https://codeberg.org/lczyk/gitgum", "gitgum"},
			wantRemote: "lczyk",
			wantDir:    "gitgum",
		},
		{
			name:       "conforming explicit dir stays quiet",
			raw:        "github.com/canonical/cbs-tools",
			dir:        "cbs-tools-2",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "cbs-tools-2"},
			wantRemote: "canonical",
			wantDir:    "cbs-tools-2",
		},
		{
			name:       "non-conforming explicit dir warns",
			raw:        "github.com/canonical/cbs-tools",
			dir:        "wrongname",
			wantArgs:   []string{"clone", "-o", "canonical", "https://github.com/canonical/cbs-tools", "wrongname"},
			wantRemote: "canonical",
			wantDir:    "wrongname",
			wantNote:   "does not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref := mustRef(t, tc.raw)
			p := buildClonePlan(ref, tc.raw, tc.dir, tc.depth)
			assert.EqualArrays(t, p.args, tc.wantArgs)
			assert.Equal(t, p.remote, tc.wantRemote)
			assert.Equal(t, p.dir, tc.wantDir)
			if tc.wantNote == "" {
				assert.Equal(t, p.note, "")
			} else {
				assert.ContainsString(t, p.note, tc.wantNote)
			}
		})
	}
}

func TestPlainClonePlan(t *testing.T) {
	t.Parallel()
	p := plainClonePlan("git@bitbucket.org:foo/bar.git", "", 0, "note")
	assert.EqualArrays(t, p.args, []string{"clone", "git@bitbucket.org:foo/bar.git"})
	assert.Equal(t, p.remote, "")

	p = plainClonePlan("../local", "dest", 2, "")
	assert.EqualArrays(t, p.args, []string{"clone", "--depth", "2", "../local", "dest"})
	assert.Equal(t, p.dir, "dest")
}

func TestResolveShorthand(t *testing.T) {
	t.Parallel()
	ref := mustRef(t, "canonical/cbs-tools")

	t.Run("single match resolves without a prompt", func(t *testing.T) {
		sel := &stubSelector{}
		c := &CloneCommand{
			cmdIO: cmdIO{UI: sel},
			probe: func(u string) bool { return u == "https://gitlab.com/canonical/cbs-tools" },
		}
		got, err := c.resolveShorthand(ref)
		require.NoError(t, err, "resolve")
		assert.Equal(t, got.Forge, git.ForgeGitLab)
		assert.Equal(t, got.Host, "gitlab.com")
		assert.Equal(t, len(sel.selectCalls), 0)
	})

	t.Run("multiple matches prompt a github-first picker", func(t *testing.T) {
		sel := &stubSelector{selectAnswers: []string{"https://codeberg.org/canonical/cbs-tools"}}
		c := &CloneCommand{
			cmdIO: cmdIO{UI: sel},
			probe: func(u string) bool { return true }, // exists everywhere
		}
		got, err := c.resolveShorthand(ref)
		require.NoError(t, err, "resolve")
		assert.Equal(t, got.Forge, git.ForgeCodeberg)
		require.Equal(t, len(sel.selectCalls), 1)
		// options are the candidate urls in preference order (github first).
		assert.EqualArrays(t, sel.selectCalls[0].Options, []string{
			"https://github.com/canonical/cbs-tools",
			"https://gitlab.com/canonical/cbs-tools",
			"https://codeberg.org/canonical/cbs-tools",
		})
	})

	t.Run("no match errors", func(t *testing.T) {
		c := &CloneCommand{probe: func(u string) bool { return false }}
		_, err := c.resolveShorthand(ref)
		require.Error(t, err, "not found")
	})
}
