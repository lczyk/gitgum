package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestParseSinceArg(t *testing.T) {
	cases := map[string]struct {
		in        string
		wantSince string
		wantCount int
		wantErr   bool
	}{
		"empty":       {"", "", 0, false},
		"shorthand w": {"2w", "2 weeks ago", 0, false},
		"shorthand d": {"10d", "10 days ago", 0, false},
		"shorthand h": {"1h", "1 hours ago", 0, false},
		"shorthand m": {"30m", "30 minutes ago", 0, false},
		"shorthand s": {"45s", "45 seconds ago", 0, false},
		"shorthand y": {"3y", "3 years ago", 0, false},
		"iso date":    {"2024-01-01", "2024-01-01", 0, false},
		"iso datetime": {
			"2024-01-01T12:00:00", "2024-01-01T12:00:00", 0, false,
		},
		"depth":            {"4", "", 4, false},
		"zero depth":       {"0", "", 0, true},
		"negative depth":   {"-1", "", 0, true},
		"unknown unit":     {"2x", "", 0, true},
		"junk":             {"yesterday", "", 0, true},
		"two weeks ago":    {"2 weeks ago", "", 0, true},
		"approxidate fail": {"2.weeks.ago", "", 0, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotSince, gotCount, err := parseSinceArg(tc.in)
			if tc.wantErr {
				assert.That(t, err != nil, "expected error for %q", tc.in)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, gotSince, tc.wantSince)
			assert.Equal(t, gotCount, tc.wantCount)
		})
	}
}

func TestHandleFollowKey(t *testing.T) {
	const screenH = 24 // page = 22

	type expected struct {
		offset   int
		tailMode bool
		alive    bool
	}

	cases := []struct {
		name      string
		startOff  int
		startTail bool
		ev        *tcell.EventKey
		want      expected
	}{
		{"j moves down + drops tail", 5, true,
			tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone),
			expected{6, false, true}},
		{"down arrow same as j", 5, true,
			tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
			expected{6, false, true}},
		{"k moves up + drops tail", 5, true,
			tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone),
			expected{4, false, true}},
		{"up arrow same as k", 5, true,
			tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone),
			expected{4, false, true}},
		{"g jumps to top + drops tail", 5, true,
			tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone),
			expected{0, false, true}},
		{"Home same as g", 5, true,
			tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone),
			expected{0, false, true}},
		{"G re-engages tail", 5, false,
			tcell.NewEventKey(tcell.KeyRune, 'G', tcell.ModNone),
			expected{5, true, true}}, // offset preserved; redraw will snap
		{"End same as G", 5, false,
			tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone),
			expected{5, true, true}},
		{"PgDn jumps page", 0, true,
			tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone),
			expected{22, false, true}},
		{"Ctrl-D same as PgDn", 0, true,
			tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone),
			expected{22, false, true}},
		{"Space pages down", 0, true,
			tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone),
			expected{22, false, true}},
		{"PgUp jumps page back", 30, false,
			tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone),
			expected{8, false, true}},
		{"Ctrl-U same as PgUp", 30, false,
			tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone),
			expected{8, false, true}},
		{"q exits", 5, false,
			tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone),
			expected{5, false, false}},
		{"Esc exits", 5, false,
			tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
			expected{5, false, false}},
		{"Ctrl-C exits", 5, false,
			tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone),
			expected{5, false, false}},
		{"unknown key no-op", 5, true,
			tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone),
			expected{5, true, true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			off := tc.startOff
			tail := tc.startTail
			alive := handleFollowKey(tc.ev, &off, &tail, screenH)
			assert.Equal(t, off, tc.want.offset)
			assert.Equal(t, tail, tc.want.tailMode)
			assert.Equal(t, alive, tc.want.alive)
		})
	}
}

// page-size clamp: tiny screens fall back to page=1 so PgDn / PgUp don't
// stall on weird geometries.
func TestHandleFollowKey_TinyScreen(t *testing.T) {
	off := 0
	tail := false
	handleFollowKey(tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone), &off, &tail, 1)
	assert.Equal(t, off, 1) // page clamped to 1
}

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
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "Add A")
	temp_repo.RunGit(t, dir, "checkout", "-b", "feature")
	temp_repo.CreateCommit(t, dir, "b.txt", "b\n", "Add B on feature")
	temp_repo.RunGit(t, dir, "checkout", "main")
	temp_repo.CreateCommit(t, dir, "c.txt", "c\n", "Add C on main")
	repo := git.Repo{Dir: dir}

	t.Run("empty since shows full history across all branches", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}}
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
		t.Parallel()
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}}
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
		t.Parallel()
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}, Since: "2000-01-01"}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := buf.String()
		assert.ContainsString(t, out, "Add A")
		assert.ContainsString(t, out, "Add B on feature")
		assert.ContainsString(t, out, "Add C on main")
	})

	t.Run("future iso since shows no commits", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}, Since: "2099-01-01"}
		err := cmd.Execute(nil)
		assert.NoError(t, err)

		out := strings.TrimSpace(buf.String())
		assert.That(t, !strings.Contains(out, "Add A"), "should not contain commits dated before 2099")
		assert.That(t, !strings.Contains(out, "Add B on feature"), "should not contain commits dated before 2099")
		assert.That(t, !strings.Contains(out, "Add C on main"), "should not contain commits dated before 2099")
	})
}

func TestTreeCommand_Reverse(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "Add A")
	temp_repo.CreateCommit(t, dir, "b.txt", "b\n", "Add B")
	temp_repo.CreateCommit(t, dir, "c.txt", "c\n", "Add C")
	repo := git.Repo{Dir: dir}

	var buf bytes.Buffer
	cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}, Reverse: true}
	err := cmd.Execute(nil)
	assert.NoError(t, err)

	out := buf.String()
	idxA := strings.Index(out, "Add A")
	idxB := strings.Index(out, "Add B")
	idxC := strings.Index(out, "Add C")
	assert.That(t, idxA >= 0 && idxB >= 0 && idxC >= 0, "all commits should appear")
	assert.That(t, idxC < idxB, "with --reverse, newest (Add C) should come before older (Add B)")
	assert.That(t, idxB < idxA, "with --reverse, Add B should come before Add A")
}

func TestTreeCommand_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "a.txt", "a\n", "Add A")
	repo := git.Repo{Dir: dir}

	var buf bytes.Buffer
	cmd := &TreeCommand{cmdIO: cmdIO{Out: &buf, Repo: repo}}
	err := cmd.Execute(nil)
	assert.NoError(t, err)
	assert.That(t, !strings.Contains(buf.String(), "\x1b"), "should not contain ansi escapes when NO_COLOR is set")
}
