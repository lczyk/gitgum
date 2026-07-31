package dirty

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
)

// group is where every truthfulness question lives, and it takes no git, so
// the cases that matter are plain data.
func TestGroup(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		unstaged, staged, untracked, ignored []string
		want                                 Plan
	}{
		"nothing": {want: Plan{}},
		"a file changed both ways counts once": {
			unstaged: []string{"both"},
			staged:   []string{"both"},
			want:     Plan{Tracked: []string{"both"}},
		},
		"both halves of a rename are at risk": {
			staged: []string{"old", "new"},
			want:   Plan{Tracked: []string{"old", "new"}},
		},
		"untracked directory arrives already expanded": {
			untracked: []string{"dir/deep/a", "dir/deep/b", "loose"},
			want:      Plan{Untracked: []string{"dir/deep/a", "dir/deep/b", "loose"}},
		},
		"ignored files are their own group": {
			untracked: []string{"loose"},
			ignored:   []string{"node_modules/x", ".env"},
			want:      Plan{Untracked: []string{"loose"}, Ignored: []string{"node_modules/x", ".env"}},
		},
		"a path reported twice across groups lands in the first": {
			untracked: []string{"shared"},
			ignored:   []string{"shared", "only-ignored"},
			want:      Plan{Untracked: []string{"shared"}, Ignored: []string{"only-ignored"}},
		},
		"empty entries are dropped": {
			unstaged: []string{"", "real", ""},
			want:     Plan{Tracked: []string{"real"}},
		},
		"paths with spaces survive intact": {
			untracked: []string{"with space", "with  two"},
			want:      Plan{Untracked: []string{"with space", "with  two"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := group(tc.unstaged, tc.staged, tc.untracked, tc.ignored)
			assert.EqualArrays(t, got.Tracked, tc.want.Tracked)
			assert.EqualArrays(t, got.Untracked, tc.want.Untracked)
			assert.EqualArrays(t, got.Ignored, tc.want.Ignored)
		})
	}
}

func TestPlanCount(t *testing.T) {
	t.Parallel()
	p := Plan{Tracked: []string{"a"}, Untracked: []string{"b", "c"}, Ignored: []string{"d", "e", "f"}}
	assert.Equal(t, p.Count(Options{Tracked: true}), 1)
	assert.Equal(t, p.Count(Options{Untracked: true}), 2)
	assert.Equal(t, p.Count(Options{Tracked: true, Untracked: true}), 3)
	assert.Equal(t, p.Count(Options{Tracked: true, Untracked: true, Ignored: true}), 6)
	assert.That(t, !p.Empty(), "plan with files should not be empty")
	assert.That(t, Plan{}.Empty(), "zero plan should be empty")
}

// fakeRepo records the argv it was asked to run and replays canned output,
// so the collector's own failure handling can be driven without a repo.
type fakeRepo struct {
	out        map[string]string
	err        error
	inProgress string
	writes     [][]string
}

func key(args []string) string {
	k := ""
	for _, a := range args {
		k += a + " "
	}
	return k
}

func (f *fakeRepo) Run(args ...string) (string, string, error) {
	if f.err != nil {
		return "", "boom", f.err
	}
	return f.out[key(args)], "", nil
}

func (f *fakeRepo) RunWrite(args ...string) (string, string, error) {
	f.writes = append(f.writes, args)
	return "", "", nil
}

func (f *fakeRepo) InProgress() (string, bool) {
	return f.inProgress, f.inProgress != ""
}

func TestDiscardRefusesMidOperation(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"a merge", "an interactive rebase", "an unresolved conflict"} {
		f := &fakeRepo{inProgress: op}
		err := Plan{Tracked: []string{"x"}}.Discard(f, Options{Tracked: true})
		assert.Error(t, err, assert.AnyError, "should refuse during %s", op)
		assert.ContainsString(t, err.Error(), op)
		assert.Equal(t, len(f.writes), 0) // nothing was run
	}
}

func TestDiscardOptions(t *testing.T) {
	t.Parallel()
	full := Plan{Tracked: []string{"a"}, Untracked: []string{"b"}, Ignored: []string{"c"}}

	t.Run("tracked only resets and does not clean", func(t *testing.T) {
		f := &fakeRepo{}
		require.NoError(t, full.Discard(f, Options{Tracked: true}))
		require.Equal(t, len(f.writes), 1)
		assert.EqualArrays(t, f.writes[0], []string{"reset", "--hard"})
	})

	t.Run("untracked cleans without -x", func(t *testing.T) {
		f := &fakeRepo{}
		require.NoError(t, full.Discard(f, Options{Untracked: true}))
		require.Equal(t, len(f.writes), 1)
		assert.EqualArrays(t, f.writes[0], []string{"clean", "-fd"})
	})

	t.Run("ignored widens the sweep and implies untracked", func(t *testing.T) {
		f := &fakeRepo{}
		require.NoError(t, full.Discard(f, Options{Ignored: true}))
		require.Equal(t, len(f.writes), 1)
		// git has no ignored-only mode, so asking for ignored alone must not
		// silently skip the sweep it rides on
		assert.EqualArrays(t, f.writes[0], []string{"clean", "-fd", "-x"})
	})

	t.Run("nothing selected runs nothing", func(t *testing.T) {
		f := &fakeRepo{}
		require.NoError(t, full.Discard(f, Options{}))
		assert.Equal(t, len(f.writes), 0)
	})

	t.Run("empty groups run nothing even when selected", func(t *testing.T) {
		f := &fakeRepo{}
		require.NoError(t, Plan{}.Discard(f, Options{Tracked: true, Untracked: true}))
		assert.Equal(t, len(f.writes), 0)
	})
}

func TestScanReportsCollectorFailure(t *testing.T) {
	t.Parallel()
	_, err := Scan(&fakeRepo{err: assert.AnyError})
	assert.Error(t, err, assert.AnyError, "scan should surface a failed listing")
	assert.ContainsString(t, err.Error(), "listing modified files")
}
