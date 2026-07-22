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
		// bare shorthand: no host, forge unknown, user/repo set
		{"canonical/cbs-tools", ForgeUnknown, "", "canonical", "cbs-tools", true},
		{"lczyk/gitgum.git", ForgeUnknown, "", "lczyk", "gitgum", true},
		// unmodelled host: parsed, forge unknown, host retained
		{"https://gitlab.example.com/team/proj", ForgeUnknown, "gitlab.example.com", "team", "proj", true},
		{"git@bitbucket.org:foo/bar.git", ForgeUnknown, "bitbucket.org", "foo", "bar", true},
		// rejects
		{"https://github.com/onlyuser", ForgeUnknown, "", "", "", false},
		{"/local/path/to/repo", ForgeUnknown, "", "", "", false},
		{"github.com", ForgeUnknown, "", "", "", false},
		{"", ForgeUnknown, "", "", "", false},
	}
	for _, c := range cases {
		ref, ok := ParseRepoRef(c.raw)
		if ok != c.ok || ref.Forge != c.forge || ref.Host != c.host || ref.User != c.user || ref.Repo != c.repo {
			t.Errorf("ParseRepoRef(%q) = (%v, host=%q, %q/%q, ok=%v), want (%v, host=%q, %q/%q, ok=%v)",
				c.raw, ref.Forge, ref.Host, ref.User, ref.Repo, ok,
				c.forge, c.host, c.user, c.repo, c.ok)
		}
	}
}

func TestRepoRefURLOn(t *testing.T) {
	t.Parallel()
	ref := RepoRef{User: "canonical", Repo: "cbs-tools"}
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
