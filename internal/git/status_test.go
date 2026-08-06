package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestStatusArgs(t *testing.T) {
	cases := map[string]struct {
		opt  ScanOpts
		want []string
	}{
		"default is normal, no ignored, no branch": {
			opt:  ScanOpts{},
			want: []string{"status", "--porcelain", "-z", "-unormal"},
		},
		"report scan": {
			opt:  ScanOpts{Branch: true},
			want: []string{"status", "--porcelain", "-z", "--branch", "-unormal"},
		},
		"discard scan": {
			opt:  ScanOpts{Untracked: UntrackedAll, Ignored: true},
			want: []string{"status", "--porcelain", "-z", "-uall", "--ignored"},
		},
		"dirty-tree scan": {
			opt:  ScanOpts{Untracked: UntrackedNone},
			want: []string{"status", "--porcelain", "-z", "-uno"},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualArrays(t, StatusArgs(tt.opt), tt.want)
		})
	}
}

// --ignored=matching would collapse an ignored directory to one entry, which
// is the whole reason a caller asks for ignored files.
func TestStatusArgs_IgnoredIsNotMatching(t *testing.T) {
	for _, a := range StatusArgs(ScanOpts{Ignored: true}) {
		assert.That(t, !strings.HasPrefix(a, "--ignored="), "got %q", a)
	}
}

func TestParseStatus(t *testing.T) {
	cases := map[string]struct {
		raw        string
		wantBranch string
		want       []Entry
	}{
		"empty": {raw: "", want: nil},
		"trailing NUL leaves no empty entry": {
			raw:  " M a\x00",
			want: []Entry{{X: ' ', Y: 'M', Path: "a"}},
		},
		"leading space is a status character": {
			raw: " M unstaged\x00M  staged\x00MM both\x00",
			want: []Entry{
				{X: ' ', Y: 'M', Path: "unstaged"},
				{X: 'M', Y: ' ', Path: "staged"},
				{X: 'M', Y: 'M', Path: "both"},
			},
		},
		"branch line comes back verbatim": {
			raw:        "## main...origin/main [ahead 1]\x00?? f\x00",
			wantBranch: "## main...origin/main [ahead 1]",
			want:       []Entry{{X: '?', Y: '?', Path: "f"}},
		},
		"detached head": {
			raw:        "## HEAD (no branch)\x00",
			wantBranch: "## HEAD (no branch)",
			want:       nil,
		},
		"rename carries both halves, destination first": {
			raw:  "R  new.go\x00old.go\x00 M other\x00",
			want: []Entry{{X: 'R', Y: ' ', Path: "new.go", RenamedFrom: "old.go"}, {X: ' ', Y: 'M', Path: "other"}},
		},
		"copy pairs the same way": {
			raw:  "C  copy.go\x00orig.go\x00",
			want: []Entry{{X: 'C', Y: ' ', Path: "copy.go", RenamedFrom: "orig.go"}},
		},
		"unmerged": {
			raw:  "UU conflict\x00",
			want: []Entry{{X: 'U', Y: 'U', Path: "conflict"}},
		},
		"paths keep spaces and quotes unescaped": {
			raw:  "?? sp ace.txt\x00?? uni\"quote.txt\x00",
			want: []Entry{{X: '?', Y: '?', Path: "sp ace.txt"}, {X: '?', Y: '?', Path: `uni"quote.txt`}},
		},
		"ignored": {
			raw:  "!! build/out.o\x00",
			want: []Entry{{X: '!', Y: '!', Path: "build/out.o"}},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			branch, got, err := ParseStatus(tt.raw)
			assert.NoError(t, err)
			assert.Equal(t, branch, tt.wantBranch)
			assert.EqualArrays(t, got, tt.want)
		})
	}
}

// A record we cannot read means a path we would not report, and every caller
// of this is describing something it is about to change.
func TestParseStatus_MalformedIsAnError(t *testing.T) {
	cases := map[string]string{
		"too short":             "M\x00",
		"no separating space":   "MMpath\x00",
		"rename without source": "R  new.go\x00",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := ParseStatus(raw)
			assert.Error(t, err, assert.AnyError)
		})
	}
}

func TestEntryPredicates(t *testing.T) {
	cases := map[string]struct {
		e                  Entry
		untracked, ignored bool
	}{
		"unstaged edit": {e: Entry{X: ' ', Y: 'M'}},
		"staged edit":   {e: Entry{X: 'M', Y: ' '}},
		"staged rename": {e: Entry{X: 'R', Y: ' '}},
		"untracked":     {e: Entry{X: '?', Y: '?'}, untracked: true},
		"ignored":       {e: Entry{X: '!', Y: '!'}, ignored: true},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.e.Untracked(), tt.untracked)
			assert.Equal(t, tt.e.Ignored(), tt.ignored)
		})
	}
}

// The flags carry most of the risk here, so they get exercised against real
// git rather than only against canned text.
func TestRepoStatus_AgainstGit(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.WriteFile(t, dir, ".gitignore", "build/\n")
	temp_repo.CreateCommit(t, dir, "tracked.txt", "a\n", "chore: seed")
	temp_repo.RunGit(t, dir, "add", ".gitignore")
	temp_repo.RunGit(t, dir, "commit", "-m", "chore: ignore")

	temp_repo.WriteFile(t, dir, "tracked.txt", "a\nb\n")
	mustMkdirAll(t, filepath.Join(dir, "untracked_dir", "nested"))
	temp_repo.WriteFile(t, dir, filepath.Join("untracked_dir", "nested", "u1.txt"), "u\n")
	temp_repo.WriteFile(t, dir, filepath.Join("untracked_dir", "nested", "u2.txt"), "u\n")
	temp_repo.WriteFile(t, dir, "sp ace.txt", "s\n")
	mustMkdirAll(t, filepath.Join(dir, "build"))
	temp_repo.WriteFile(t, dir, filepath.Join("build", "out.o"), "o\n")

	r := Repo{Dir: dir}

	branch, entries, err := r.Status(ScanOpts{Branch: true})
	assert.NoError(t, err)
	assert.That(t, strings.HasPrefix(branch, "## "), "branch line %q", branch)
	assert.EqualArraysUnordered(t, pathsOf(entries), []string{
		"tracked.txt", "untracked_dir/", "sp ace.txt",
	})

	_, entries, err = r.Status(ScanOpts{Untracked: UntrackedAll, Ignored: true})
	assert.NoError(t, err)
	assert.EqualArraysUnordered(t, pathsOf(entries), []string{
		"tracked.txt",
		"untracked_dir/nested/u1.txt",
		"untracked_dir/nested/u2.txt",
		"sp ace.txt",
		"build/out.o",
	})

	// The space survives: git would have quoted it without -z.
	for _, e := range entries {
		if e.Path == "sp ace.txt" {
			assert.That(t, e.Untracked(), "expected untracked")
		}
		if e.Path == "build/out.o" {
			assert.That(t, e.Ignored(), "expected ignored")
		}
		if e.Path == "tracked.txt" {
			assert.Equal(t, e.X, byte(' '))
			assert.Equal(t, e.Y, byte('M'))
		}
	}
}

// A staged rename must come back as both halves: a hard reset restores the
// source as well as removing the destination.
func TestRepoStatus_RenameKeepsBothHalves(t *testing.T) {
	dir := temp_repo.NewRepo(t)
	temp_repo.CreateCommit(t, dir, "old.txt", "contents worth detecting\n", "chore: seed")
	temp_repo.RunGit(t, dir, "mv", "old.txt", "new.txt")

	_, entries, err := Repo{Dir: dir}.Status(ScanOpts{})
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	if len(entries) == 1 {
		assert.Equal(t, entries[0].Path, "new.txt")
		assert.Equal(t, entries[0].RenamedFrom, "old.txt")
	}
}

func pathsOf(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}

func mustMkdirAll(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
