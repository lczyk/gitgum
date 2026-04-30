package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestSwapGraphSlashes(t *testing.T) {
	cases := map[string]struct {
		in, want string
	}{
		"plain forward slash":   {"|/  ", "|\\  "},
		"plain back slash":      {"|\\  ", "|/  "},
		"no slashes":            {"* | abc", "* | abc"},
		"slashes only in graph": {"|/ abc/def", "|\\ abc/def"},
		"ansi-wrapped slash": {
			"\x1b[32m|\x1b[m\x1b[32m/\x1b[m  ",
			"\x1b[32m|\x1b[m\x1b[32m\\\x1b[m  ",
		},
		"ansi prefix then hash": {
			"* \x1b[32m|\x1b[m \x1b[33m90a0808\x1b[m C/D",
			"* \x1b[32m|\x1b[m \x1b[33m90a0808\x1b[m C/D",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := swapGraphSlashes(tc.in)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestTreeCommand_Execute(t *testing.T) {
	dir := temp_repo.InitTempRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "Add A")
	temp_repo.RunGit(t, dir, "checkout", "-b", "feature")
	temp_repo.CreateCommit(t, dir, "b.txt", "b\n", "Add B on feature")
	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.CreateCommit(t, dir, "c.txt", "c\n", "Add C on main")

	t.Run("empty since shows full history across all branches", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf}}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := buf.String()
		assert.That(t, strings.Contains(out, "*"), "should contain graph node markers")
		assert.ContainsString(t, out, "Add A")
		assert.ContainsString(t, out, "Add B on feature")
		assert.ContainsString(t, out, "Add C on main")
		assert.ContainsString(t, out, "main")
		assert.ContainsString(t, out, "feature")
	})

	t.Run("output is reversed: oldest at top, newest at bottom", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf}}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := buf.String()
		idxInitial := strings.Index(out, "initial commit")
		idxA := strings.Index(out, "Add A")
		idxC := strings.Index(out, "Add C on main")
		assert.That(t, idxInitial >= 0 && idxA >= 0 && idxC >= 0, "all three commits should appear")
		assert.That(t, idxInitial < idxA, "initial commit should come before Add A (oldest first)")
		assert.That(t, idxA < idxC, "Add A should come before Add C on main (newest last)")
	})

	t.Run("ancient since shows full history", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf}, Since: "1970-01-01"}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := buf.String()
		assert.ContainsString(t, out, "Add A")
		assert.ContainsString(t, out, "Add B on feature")
		assert.ContainsString(t, out, "Add C on main")
	})

	t.Run("future since shows no commits", func(t *testing.T) {
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf}, Since: "2099-01-01"}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := strings.TrimSpace(buf.String())
		assert.That(t, !strings.Contains(out, "Add A"), "should not contain commits dated before 2099")
		assert.That(t, !strings.Contains(out, "Add B on feature"), "should not contain commits dated before 2099")
		assert.That(t, !strings.Contains(out, "Add C on main"), "should not contain commits dated before 2099")
	})
}
