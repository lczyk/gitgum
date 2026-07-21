package git

import "testing"

func TestParseGitHubURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw        string
		user, repo string
		ok         bool
	}{
		{"https://github.com/nsklikas/docker-snap", "nsklikas", "docker-snap", true},
		{"https://github.com/nsklikas/docker-snap.git", "nsklikas", "docker-snap", true},
		{"http://github.com/lczyk/gitgum", "lczyk", "gitgum", true},
		{"git@github.com:nsklikas/docker-snap.git", "nsklikas", "docker-snap", true},
		{"git@github.com:nsklikas/docker-snap", "nsklikas", "docker-snap", true},
		{"ssh://git@github.com/lczyk/gitgum.git", "lczyk", "gitgum", true},
		{"git://github.com/lczyk/gitgum.git", "lczyk", "gitgum", true},
		{"https://github.com/lczyk/gitgum/", "lczyk", "gitgum", true},
		{"https://GitHub.com/lczyk/gitgum", "lczyk", "gitgum", true}, // host case-insensitive
		// non-github / unparseable
		{"https://gitlab.com/lczyk/gitgum", "", "", false},
		{"git@bitbucket.org:foo/bar.git", "", "", false},
		{"/local/path/to/repo", "", "", false},
		{"https://github.com/onlyuser", "", "", false},
		{"", "", "", false},
		{"github.com", "", "", false},
	}
	for _, c := range cases {
		user, repo, ok := ParseGitHubURL(c.raw)
		if ok != c.ok || user != c.user || repo != c.repo {
			t.Errorf("ParseGitHubURL(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.raw, user, repo, ok, c.user, c.repo, c.ok)
		}
	}
}
