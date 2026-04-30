package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

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
