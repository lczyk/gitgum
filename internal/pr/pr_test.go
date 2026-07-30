package pr

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
)

func TestBranchNameAndFetchRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, BranchName("origin", 51), "pr/origin/51")
	assert.Equal(t, BranchName("upstream", 7), "pr/upstream/7")
	assert.Equal(t, Meta{Number: 51, Type: "head"}.FetchRef(git.ForgeGitHub), "refs/pull/51/head")
	assert.Equal(t, Meta{Number: 9, Type: "merge"}.FetchRef(git.ForgeGitHub), "refs/pull/9/merge")
	// gitea copies github's namespace; gitlab has its own.
	assert.Equal(t, Meta{Number: 7, Type: "head"}.FetchRef(git.ForgeCodeberg), "refs/pull/7/head")
	assert.Equal(t, Meta{Number: 12, Type: "head"}.FetchRef(git.ForgeGitLab), "refs/merge-requests/12/head")
}

func TestParseBranchName(t *testing.T) {
	t.Parallel()
	remote, number, ok := ParseBranchName("pr/upstream/72")
	require.That(t, ok, "pr/upstream/72 should parse")
	assert.Equal(t, remote, "upstream")
	assert.Equal(t, number, 72)

	for _, b := range []string{"main", "feat/foo", "pr/origin/notanumber", "pr/origin"} {
		if _, _, ok := ParseBranchName(b); ok {
			t.Errorf("ParseBranchName(%q) = ok, want not ok", b)
		}
	}
}

func TestReadMeta_FromConfig(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	want := Meta{Remote: "upstream", Number: 72, Type: "merge"}
	require.NoError(t, WriteMeta(r, "pr/upstream/72", want))

	got, ok, err := ReadMeta(r, "pr/upstream/72")
	require.NoError(t, err)
	require.That(t, ok, "config-backed PR meta should be found")
	assert.Equal(t, got, want)
}

func TestReadMeta_NameFallback(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	// No config written -- identity derived from the pr/<remote>/<number> name,
	// defaulting to the head ref.
	got, ok, err := ReadMeta(r, "pr/origin/5")
	require.NoError(t, err)
	require.That(t, ok, "name should be parsed as a fallback")
	assert.Equal(t, got, Meta{Remote: "origin", Number: 5, Type: "head"})
}

func TestReadMeta_NotAPRBranch(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	r := git.Repo{Dir: dir}

	for _, b := range []string{"main", "feat/foo", "pr/origin/notanumber", "pr/origin"} {
		_, ok, err := ReadMeta(r, b)
		require.NoError(t, err)
		assert.That(t, !ok, "%q should not be a PR branch", b)
	}
}

func TestParseRefs(t *testing.T) {
	cases := map[string]struct {
		forge    git.Forge
		input    string
		expected []Ref
	}{
		"empty output": {input: "", expected: []Ref{}},
		"single head ref": {
			input:    "abc123def456\trefs/pull/42/head",
			expected: []Ref{{Number: 42, Type: "head"}},
		},
		"single merge ref": {
			input:    "abc123def456\trefs/pull/10/merge",
			expected: []Ref{{Number: 10, Type: "merge"}},
		},
		"head wins over merge for same PR": {
			input:    "aaa\trefs/pull/5/merge\nbbb\trefs/pull/5/head",
			expected: []Ref{{Number: 5, Type: "head"}},
		},
		"head already present ignores later merge": {
			input:    "aaa\trefs/pull/5/head\nbbb\trefs/pull/5/merge",
			expected: []Ref{{Number: 5, Type: "head"}},
		},
		"multiple PRs sorted descending": {
			input: "aaa\trefs/pull/1/head\nbbb\trefs/pull/99/merge\nccc\trefs/pull/50/head",
			expected: []Ref{
				{Number: 99, Type: "merge"},
				{Number: 50, Type: "head"},
				{Number: 1, Type: "head"},
			},
		},
		"non-PR refs ignored": {
			input:    "aaa\trefs/heads/main\nbbb\trefs/tags/v1.0\nccc\trefs/pull/7/head",
			expected: []Ref{{Number: 7, Type: "head"}},
		},
		// each forge is read only in its own namespace, so a remote that
		// happened to carry both would not double-count
		"gitlab reads merge requests": {
			forge:    git.ForgeGitLab,
			input:    "aaa\trefs/merge-requests/12/head\nbbb\trefs/pull/9/head",
			expected: []Ref{{Number: 12, Type: "head"}},
		},
		"github ignores merge requests": {
			forge:    git.ForgeGitHub,
			input:    "aaa\trefs/merge-requests/12/head",
			expected: []Ref{},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.EqualArrays(t, ParseRefs(tt.forge, tt.input), tt.expected)
		})
	}
}
