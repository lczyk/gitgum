package git

import "testing"

func TestParseRepoRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw   string
		forge Forge
		host  string
		user  string
		repo  string
		ok    bool
	}{
		// full urls, all forges
		{"https://github.com/lczyk/gitgum", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"https://github.com/lczyk/gitgum.git", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"http://github.com/lczyk/gitgum/", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"git@github.com:lczyk/gitgum.git", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"ssh://git@github.com/lczyk/gitgum.git", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"git://github.com/lczyk/gitgum.git", ForgeGitHub, "github.com", "lczyk", "gitgum", true},
		{"https://gitlab.com/canonical/cbs-tools", ForgeGitLab, "gitlab.com", "canonical", "cbs-tools", true},
		{"git@gitlab.com:canonical/cbs-tools.git", ForgeGitLab, "gitlab.com", "canonical", "cbs-tools", true},
		{"https://codeberg.org/lczyk/gitgum", ForgeCodeberg, "codeberg.org", "lczyk", "gitgum", true},
		// host shorthand (no scheme) + www + case-fold
		{"github.com/canonical/cbs-tools", ForgeGitHub, "github.com", "canonical", "cbs-tools", true},
		{"www.github.com/canonical/cbs-tools", ForgeGitHub, "github.com", "canonical", "cbs-tools", true},
		{"WWW.GitHub.com/canonical/cbs-tools", ForgeGitHub, "github.com", "canonical", "cbs-tools", true},
		// forge-name shorthand: leading forge NAME (not host) sets the forge
		{"github/canonical/rust-rock", ForgeGitHub, "github.com", "canonical", "rust-rock", true},
		{"gitlab/canonical/cbs-tools", ForgeGitLab, "gitlab.com", "canonical", "cbs-tools", true},
		{"codeberg/lczyk/gitgum.git", ForgeCodeberg, "codeberg.org", "lczyk", "gitgum", true},
		{"GitHub/canonical/rust-rock", ForgeGitHub, "github.com", "canonical", "rust-rock", true},
		// short forge aliases
		{"gh/rockcrafters/bonsai-rock", ForgeGitHub, "github.com", "rockcrafters", "bonsai-rock", true},
		{"gl/canonical/cbs-tools", ForgeGitLab, "gitlab.com", "canonical", "cbs-tools", true},
		{"cb/lczyk/gitgum", ForgeCodeberg, "codeberg.org", "lczyk", "gitgum", true},
		// forge name with only two segments stays bare shorthand (user "github")
		{"github/gitgum", ForgeUnknown, "", "github", "gitgum", true},
		// forge name with a deeper path is not a forge prefix -> rejected
		{"github/canonical/rust-rock/extra", ForgeUnknown, "", "", "", false},
		// a non-forge leading token with 3 segments is still a rejected deep path
		{"notaforge/canonical/rust-rock", ForgeUnknown, "", "", "", false},
		// bare shorthand: no host, forge unknown, user/repo set
		{"canonical/cbs-tools", ForgeUnknown, "", "canonical", "cbs-tools", true},
		{"lczyk/gitgum.git", ForgeUnknown, "", "lczyk", "gitgum", true},
		// unmodelled host: parsed, forge unknown, host retained
		{"https://gitlab.example.com/team/proj", ForgeUnknown, "gitlab.example.com", "team", "proj", true},
		{"git@bitbucket.org:foo/bar.git", ForgeUnknown, "bitbucket.org", "foo", "bar", true},
		// rejects
		{"https://github.com/onlyuser", ForgeUnknown, "", "", "", false},
		{"/local/path/to/repo", ForgeUnknown, "", "", "", false},
		// local paths whose leading segment has a dot must not masquerade as a host
		{"../some/local/repo", ForgeUnknown, "", "", "", false},
		{"./foo/bar/baz", ForgeUnknown, "", "", "", false},
		{"github.com", ForgeUnknown, "", "", "", false},
		{"", ForgeUnknown, "", "", "", false},
	}
	for _, c := range cases {
		ref, ok := ParseRepoRef(c.raw)
		if ok != c.ok || ref.Forge != c.forge || ref.Host != c.host || ref.Owner() != c.user || ref.Repo() != c.repo {
			t.Errorf("ParseRepoRef(%q) = (%v, host=%q, %q/%q, ok=%v), want (%v, host=%q, %q/%q, ok=%v)",
				c.raw, ref.Forge, ref.Host, ref.Owner(), ref.Repo(), ok,
				c.forge, c.host, c.user, c.repo, c.ok)
		}
	}
}

func TestParseForgeURLRoute(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw    string
		path   string
		kind   RouteKind
		number int
		ref    string
	}{
		// github and gitea host a project at exactly two segments, so anything
		// deeper is a route regardless of whether gg knows the name.
		{"https://github.com/ml-explore/mlx", "ml-explore/mlx", RouteNone, 0, ""},
		{"https://github.com/ml-explore/mlx/pull/3161", "ml-explore/mlx", RoutePR, 3161, ""},
		{"https://github.com/ml-explore/mlx/pull/3161/files", "ml-explore/mlx", RoutePR, 3161, ""},
		{"https://github.com/ml-explore/mlx/tree/main", "ml-explore/mlx", RouteTree, 0, "main"},
		{"https://github.com/ml-explore/mlx/tree/feat/x", "ml-explore/mlx", RouteTree, 0, "feat/x"},
		{"https://github.com/ml-explore/mlx/blob/main/a/b.py", "ml-explore/mlx", RouteTree, 0, "main/a/b.py"},
		{"https://github.com/ml-explore/mlx/commit/abc1234", "ml-explore/mlx", RouteCommit, 0, "abc1234"},
		{"https://github.com/ml-explore/mlx/issues/5", "ml-explore/mlx", RouteOther, 0, ""},
		{"https://codeberg.org/lczyk/gitgum/pulls/7", "lczyk/gitgum", RoutePR, 7, ""},
		{"https://codeberg.org/lczyk/gitgum/src/branch/main", "lczyk/gitgum", RouteTree, 0, "main"},
		// gitlab nests projects under subgroups and marks the boundary itself,
		// so without a "/-/" every segment belongs to the project.
		{"https://gitlab.com/group/subgroup/project", "group/subgroup/project", RouteNone, 0, ""},
		{"https://gitlab.com/group/subgroup/project/-/merge_requests/12", "group/subgroup/project", RoutePR, 12, ""},
		{"https://gitlab.com/group/project/-/tree/feat/x", "group/project", RouteTree, 0, "feat/x"},
		// an unmodelled host has no routing scheme gg may assume
		{"https://git.example.com/a/b/c", "a/b/c", RouteNone, 0, ""},
	}
	for _, c := range cases {
		got, ok := ParseForgeURL(c.raw)
		if !ok {
			t.Errorf("ParseForgeURL(%q) = not ok, want ok", c.raw)
			continue
		}
		if got.Ref.Path != c.path || got.Route.Kind != c.kind ||
			got.Route.Number != c.number || got.Route.Ref != c.ref {
			t.Errorf("ParseForgeURL(%q) = path=%q kind=%v number=%d ref=%q, want path=%q kind=%v number=%d ref=%q",
				c.raw, got.Ref.Path, got.Route.Kind, got.Route.Number, got.Route.Ref,
				c.path, c.kind, c.number, c.ref)
		}
	}
}

func TestRepoRefWithOwner(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"canonical/cbs-tools": "lczyk/cbs-tools",
		// a fork lands directly under its new owner, not at the upstream's depth
		"group/subgroup/project": "lczyk/project",
	}
	for path, want := range cases {
		if got := (RepoRef{Path: path}).WithOwner("lczyk").Path; got != want {
			t.Errorf("RepoRef{Path: %q}.WithOwner(\"lczyk\") = %q, want %q", path, got, want)
		}
	}
}

func TestRepoRefURLOn(t *testing.T) {
	t.Parallel()
	ref := RepoRef{Path: "canonical/cbs-tools"}
	cases := map[Forge]string{
		ForgeGitHub:   "https://github.com/canonical/cbs-tools",
		ForgeGitLab:   "https://gitlab.com/canonical/cbs-tools",
		ForgeCodeberg: "https://codeberg.org/canonical/cbs-tools",
	}
	for f, want := range cases {
		if got := ref.URLOn(f); got != want {
			t.Errorf("URLOn(%v) = %q, want %q", f, got, want)
		}
	}
}

func TestForgeFromName(t *testing.T) {
	t.Parallel()
	cases := map[string]Forge{
		"github":     ForgeGitHub,
		"GitHub":     ForgeGitHub,
		" gitlab ":   ForgeGitLab,
		"codeberg":   ForgeCodeberg,
		"gh":         ForgeGitHub,
		"GL":         ForgeGitLab,
		"cb":         ForgeCodeberg,
		"github.com": ForgeUnknown, // a host is not a name
		"bitbucket":  ForgeUnknown,
		"":           ForgeUnknown,
	}
	for name, want := range cases {
		if got := ForgeFromName(name); got != want {
			t.Errorf("ForgeFromName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestForgeShorthand(t *testing.T) {
	t.Parallel()
	bare, _ := ParseRepoRef("canonical/cbs-tools")
	if !bare.Shorthand() {
		t.Errorf("bare user/repo should be Shorthand()")
	}
	full, _ := ParseRepoRef("github.com/canonical/cbs-tools")
	if full.Shorthand() {
		t.Errorf("host url should not be Shorthand()")
	}
}
