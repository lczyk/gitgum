package commands

import (
	"slices"
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/assert/require"
	"github.com/lczyk/gitgum/internal/git"
	"github.com/lczyk/gitgum/internal/testutil/temp_repo"
	"github.com/lczyk/gitgum/internal/ui"
)

// resolveURL turns each accepted spelling into the url the remote will use:
// bare shorthand probes forges; a real url is kept verbatim; a bare host/forge
// spelling is canonicalised to https.
func TestAddRemoteResolveURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		probeHit   string // url the probe reports as existing (shorthand only)
		wantURL    string
		wantSelect int // expected picker calls
	}{
		{
			name:    "forge-name shorthand canonicalises to https",
			raw:     "github/canonical/rust-rock",
			wantURL: "https://github.com/canonical/rust-rock",
		},
		{
			name:    "host shorthand canonicalises to https",
			raw:     "github.com/canonical/cbs-tools",
			wantURL: "https://github.com/canonical/cbs-tools",
		},
		{
			name:    "full https url kept verbatim",
			raw:     "https://gitlab.com/canonical/cbs-tools",
			wantURL: "https://gitlab.com/canonical/cbs-tools",
		},
		{
			name:    "ssh url is not downgraded to https",
			raw:     "git@github.com:canonical/cbs-tools.git",
			wantURL: "git@github.com:canonical/cbs-tools.git",
		},
		{
			name:    "unmodelled host kept verbatim",
			raw:     "https://gitlab.example.com/team/proj",
			wantURL: "https://gitlab.example.com/team/proj",
		},
		{
			name:     "bare shorthand resolves via probe",
			raw:      "canonical/cbs-tools",
			probeHit: "https://gitlab.com/canonical/cbs-tools",
			wantURL:  "https://gitlab.com/canonical/cbs-tools",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := git.ParseRepoRef(tc.raw)
			require.That(t, ok, "ParseRepoRef(%q)", tc.raw)

			sel := &stubSelector{}
			cmd := &AddRemoteCommand{
				cmdIO: cmdIO{UI: sel},
				probe: func(u string) bool { return u == tc.probeHit },
			}
			cmd.Args.Remote = tc.raw

			got, err := cmd.resolveURL(ref)
			require.NoError(t, err, "resolveURL")
			assert.Equal(t, got, tc.wantURL)
			assert.Equal(t, len(sel.selectCalls), tc.wantSelect)
		})
	}
}

// A bare shorthand that no forge has is an error, no remote is touched.
func TestAddRemoteResolveURL_ShorthandNotFound(t *testing.T) {
	t.Parallel()
	ref, ok := git.ParseRepoRef("nobody/nothing")
	require.That(t, ok, "ParseRepoRef")

	cmd := &AddRemoteCommand{probe: func(string) bool { return false }}
	cmd.Args.Remote = "nobody/nothing"
	_, err := cmd.resolveURL(ref)
	require.Error(t, err, "not found")
}

// Local paths are rejected before any remote work.
func TestAddRemoteCommand_RejectsLocalPath(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	cmd := &AddRemoteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	cmd.Args.Remote = "../some/local/repo"

	err := cmd.Execute(nil)
	require.Error(t, err, "not a resolvable repo ref")
	assert.ContainsString(t, err.Error(), "not a resolvable repo ref")
}

// Re-adding a remote that already points at the resolved url is a quiet no-op
// (no error, no duplicate, no fetch).
func TestAddRemoteCommand_IdenticalURLIsNoop(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/widget")

	var out strings.Builder
	cmd := &AddRemoteCommand{cmdIO: cmdIO{Out: &out, Repo: git.Repo{Dir: dir}}}
	cmd.Args.Remote = "github.com/canonical/widget" // canonicalises to the same url

	require.NoError(t, cmd.Execute(nil))
	assert.ContainsString(t, out.String(), "nothing to do")

	remotes, err := git.Repo{Dir: dir}.GetRemotes()
	require.NoError(t, err, "list remotes")
	assert.EqualArrays(t, remotes, []string{"canonical"})
}

// A name collision with a different url errors and points at set-url; the
// existing remote is left untouched.
func TestAddRemoteCommand_NameCollisionErrors(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/other")

	cmd := &AddRemoteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	cmd.Args.Remote = "github.com/canonical/widget"

	err := cmd.Execute(nil)
	require.Error(t, err, "already exists")
	assert.ContainsString(t, err.Error(), "already exists")
	assert.ContainsString(t, err.Error(), "set-url")

	// existing remote url is unchanged.
	got, err := git.Repo{Dir: dir}.RemoteURL("canonical")
	require.NoError(t, err, "get-url")
	assert.Equal(t, got, "https://github.com/canonical/other")
}

// When the post-add fetch fails, the freshly-added remote is rolled back so no
// half-configured remote is left behind. Uses a reserved .invalid host that
// never resolves, so the fetch fails fast and offline.
func TestAddRemoteCommand_FetchFailureRollsBack(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &AddRemoteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	cmd.Args.Remote = "https://team.invalid/nobody/nope"

	err := cmd.Execute(nil)
	require.Error(t, err, "removed it again")
	assert.ContainsString(t, err.Error(), "removed it again")

	remotes, err := git.Repo{Dir: dir}.GetRemotes()
	require.NoError(t, err, "list remotes")
	assert.Equal(t, len(remotes), 0) // rolled back
}

// A lone user token borrows the repo name and forge from the existing remotes,
// which is how "add my fork" spells out.
func TestAddRemoteRefFromExistingRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/sd-dev")

	sel := &stubSelector{}
	cmd := &AddRemoteCommand{cmdIO: cmdIO{UI: sel, Repo: git.Repo{Dir: dir}}}
	ref, err := cmd.refFromExistingRemotes("lczyk")
	require.NoError(t, err, "refFromExistingRemotes")
	assert.Equal(t, ref.URL(), "https://github.com/lczyk/sd-dev")
	assert.Equal(t, len(sel.selectCalls), 0)
}

// With no remotes at all there is nothing to borrow a repo name from.
func TestAddRemoteRefFromExistingRemotes_NoRemotes(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)

	cmd := &AddRemoteCommand{cmdIO: cmdIO{Repo: git.Repo{Dir: dir}}}
	_, err := cmd.refFromExistingRemotes("lczyk")
	require.Error(t, err, "bare user")
	assert.ContainsString(t, err.Error(), "bare user")
}

// Several remotes that name the same repo -- different owners, or the same one
// spelled ssh and https -- collapse to a single candidate, so no picker runs.
func TestAddRemoteRefFromExistingRemotes_Collapses(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "upstream", "https://github.com/canonical/sd-dev")
	temp_repo.RunGit(t, dir, "remote", "add", "origin", "git@github.com:someone/sd-dev.git")

	sel := &stubSelector{}
	cmd := &AddRemoteCommand{cmdIO: cmdIO{UI: sel, Repo: git.Repo{Dir: dir}}}
	ref, err := cmd.refFromExistingRemotes("lczyk")
	require.NoError(t, err, "refFromExistingRemotes")
	assert.Equal(t, ref.URL(), "https://github.com/lczyk/sd-dev")
	assert.Equal(t, len(sel.selectCalls), 0)
}

// Remotes naming genuinely different repos leave the choice open, so the user
// picks which one the bare user should own.
func TestAddRemoteRefFromExistingRemotes_Picks(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/sd-dev")
	temp_repo.RunGit(t, dir, "remote", "add", "other", "https://gitlab.com/canonical/cbs-tools")

	const want = "https://gitlab.com/lczyk/cbs-tools"
	sel := &stubSelector{selectAnswers: []string{want}}
	cmd := &AddRemoteCommand{cmdIO: cmdIO{UI: sel, Repo: git.Repo{Dir: dir}}}
	ref, err := cmd.refFromExistingRemotes("lczyk")
	require.NoError(t, err, "refFromExistingRemotes")
	assert.Equal(t, ref.URL(), want)

	require.Equal(t, len(sel.selectCalls), 1)
	opts := sel.selectCalls[0].Options
	assert.Equal(t, len(opts), 2)
	assert.That(t, slices.Contains(opts, want), "picker offers the gitlab candidate, got", opts)
	assert.That(t, slices.Contains(opts, "https://github.com/lczyk/sd-dev"), "picker offers the github candidate, got", opts)
}

// Cancelling the picker aborts instead of silently adding a remote.
func TestAddRemoteRefFromExistingRemotes_PickerCancelled(t *testing.T) {
	t.Parallel()
	dir := temp_repo.NewRepo(t)
	temp_repo.RunGit(t, dir, "remote", "add", "canonical", "https://github.com/canonical/sd-dev")
	temp_repo.RunGit(t, dir, "remote", "add", "other", "https://gitlab.com/canonical/cbs-tools")

	sel := &stubSelector{selectErrs: []error{ui.ErrCancelled}}
	cmd := &AddRemoteCommand{cmdIO: cmdIO{UI: sel, Repo: git.Repo{Dir: dir}}}
	_, err := cmd.refFromExistingRemotes("lczyk")
	require.Error(t, err, "aborted")
	assert.ContainsString(t, err.Error(), "aborted")
}
