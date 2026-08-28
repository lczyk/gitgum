package commands

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestFormatHeadLine_NoColor(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		in   string
		sha  string
		want string
	}{
		"same-name upstream":     {"## main...origin/main", "", "* (origin/)main"},
		"with ahead":             {"## main...origin/main [ahead 7]", "", "* (origin/)main [ahead 7]"},
		"slashed branch":         {"## feat/x...origin/feat/x [behind 2]", "", "* (origin/)feat/x [behind 2]"},
		"different upstream":     {"## main...origin/other [ahead 1]", "", "* main...origin/other [ahead 1]"},
		"no upstream":            {"## main", "", "* main"},
		"detached":               {"## HEAD (no branch)", "", "* HEAD (no branch)"},
		"no commits yet":         {"## No commits yet on main", "", "* No commits yet on main"},
		"not a head line at all": {"garbage", "", "garbage"},

		"sha, same-name upstream": {"## main...origin/main", "1a2b3c4", "* (origin/)main 1a2b3c4"},
		"sha sits before ahead":   {"## main...origin/main [ahead 7]", "1a2b3c4", "* (origin/)main 1a2b3c4 [ahead 7]"},
		"sha, no upstream":        {"## main", "1a2b3c4", "* main 1a2b3c4"},
		"sha, other upstream":     {"## main...origin/other [ahead 1]", "1a2b3c4", "* main...origin/other 1a2b3c4 [ahead 1]"},
		// an unborn HEAD has no ref to read a hash off, so there is never one
		// to print; passing one anyway must not invent a commit that is not there.
		"no commits yet ignores a sha": {"## No commits yet on main", "1a2b3c4", "* No commits yet on main"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, formatHeadLine(tt.in, tt.sha, false), tt.want)
		})
	}
}

func TestFormatHeadLine_Color(t *testing.T) {
	t.Parallel()
	got := formatHeadLine("## main...origin/main [ahead 7]", "1a2b3c4", true)
	assert.ContainsString(t, got, ansiBoldCyan+"*"+ansiReset)
	assert.ContainsString(t, got, ansiBoldRed+"origin/"+ansiReset)
	assert.ContainsString(t, got, ansiBoldGreen+"main"+ansiReset)
	assert.ContainsString(t, got, ansiYellow+"1a2b3c4"+ansiReset)
	assert.ContainsString(t, got, ansiBoldYellow+"[ahead 7]"+ansiReset)
	assert.Equal(t, stripAnsi(got), "* (origin/)main 1a2b3c4 [ahead 7]")
}

func TestFormatHeadLine_DetachedColor(t *testing.T) {
	t.Parallel()
	got := formatHeadLine("## HEAD (no branch)", "", true)
	assert.ContainsString(t, got, ansiBoldCyan+"HEAD (no branch)"+ansiReset)
}
