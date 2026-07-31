package dirty

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
)

func entry(x, y byte, path string) git.Entry { return git.Entry{X: x, Y: y, Path: path} }

// group is where every truthfulness question lives, and it takes no git, so
// the cases that matter are plain data.
func TestGroup(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		entries []git.Entry
		want    Plan
	}{
		"nothing": {want: Plan{}},
		"a file changed both ways counts once": {
			entries: []git.Entry{entry('M', 'M', "both")},
			want:    Plan{Tracked: []string{"both"}},
		},
		"both halves of a rename are at risk": {
			entries: []git.Entry{{X: 'R', Y: ' ', Path: "new", RenamedFrom: "old"}},
			want:    Plan{Tracked: []string{"new", "old"}},
		},
		"untracked directory arrives already expanded": {
			entries: []git.Entry{
				entry('?', '?', "dir/deep/a"), entry('?', '?', "dir/deep/b"), entry('?', '?', "loose"),
			},
			want: Plan{Untracked: []string{"dir/deep/a", "dir/deep/b", "loose"}},
		},
		"ignored files are their own group": {
			entries: []git.Entry{
				entry('?', '?', "loose"), entry('!', '!', "node_modules/x"), entry('!', '!', ".env"),
			},
			want: Plan{Untracked: []string{"loose"}, Ignored: []string{"node_modules/x", ".env"}},
		},
		"a path reported twice across groups lands in the first": {
			entries: []git.Entry{
				entry('?', '?', "shared"), entry('!', '!', "shared"), entry('!', '!', "only-ignored"),
			},
			want: Plan{Untracked: []string{"shared"}, Ignored: []string{"only-ignored"}},
		},
		"empty paths are dropped": {
			entries: []git.Entry{entry('M', ' ', ""), entry('M', ' ', "real")},
			want:    Plan{Tracked: []string{"real"}},
		},
		"paths with spaces survive intact": {
			entries: []git.Entry{entry('?', '?', "with space"), entry('?', '?', "with  two")},
			want:    Plan{Untracked: []string{"with space", "with  two"}},
		},
		"an unmerged path is tracked": {
			entries: []git.Entry{entry('U', 'U', "conflict")},
			want:    Plan{Tracked: []string{"conflict"}},
		},
		// Over-reporting is the safe direction: a record this package cannot
		// place must still show up somewhere.
		"an unrecognised record still lands in tracked": {
			entries: []git.Entry{entry('Z', 'Z', "strange")},
			want:    Plan{Tracked: []string{"strange"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := group(tc.entries)
			assert.EqualArrays(t, got.Tracked, tc.want.Tracked)
			assert.EqualArrays(t, got.Untracked, tc.want.Untracked)
			assert.EqualArrays(t, got.Ignored, tc.want.Ignored)
		})
	}
}

// Codes decorate, so they never gain or lose a path -- but a rename's halves
// are told apart, since which half you are looking at is the interesting part.
func TestGroupCodes(t *testing.T) {
	t.Parallel()
	got := group([]git.Entry{
		entry('M', 'M', "both"),
		{X: 'R', Y: ' ', Path: "new", RenamedFrom: "old"},
		entry('?', '?', "loose"),
		entry('!', '!', "junk"),
	})
	assert.EqualMaps(t, got.Codes, map[string]string{
		"both": "MM", "new": "R>", "old": "R<", "loose": "??", "junk": "!!",
	})
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

// fakeRepo replays a canned scan and records the argv it was asked to write,
// so the plan's own failure handling can be driven without a repo.
type fakeRepo struct {
	entries    []git.Entry
	scanned    []git.ScanOpts
	err        error
	inProgress string
	writes     [][]string
}

func (f *fakeRepo) Status(opt git.ScanOpts) (string, []git.Entry, error) {
	f.scanned = append(f.scanned, opt)
	if f.err != nil {
		return "", nil, f.err
	}
	return "", f.entries, nil
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
	assert.ContainsString(t, err.Error(), "listing the working tree")
}

// A directory standing in for its contents, or an ignored file left unlisted,
// would each report less than a discard destroys.
func TestScanAsksForEveryFile(t *testing.T) {
	t.Parallel()
	f := &fakeRepo{}
	_, err := Scan(f)
	require.NoError(t, err)
	require.Equal(t, len(f.scanned), 1)
	assert.Equal(t, f.scanned[0].Untracked, git.UntrackedAll)
	assert.That(t, f.scanned[0].Ignored, "ignored files must be listed")
}
