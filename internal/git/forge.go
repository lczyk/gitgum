package git

import "strings"

// Forge is a git hosting provider gg knows how to reason about: it can name a
// remote after the user/org, reconstruct a clone url, and (for clone) probe
// whether a repo exists there. ForgeUnknown covers every host we don't model
// (self-hosted gitlab, bitbucket, sourcehut, a local path, ...).
//
// This is the single home for "which forge is this url?" categorisation --
// parsing (ParseRepoRef), naming (checkRemoteNaming), and clone all route
// through here rather than string-matching "github.com" in scattered places.
type Forge int

const (
	ForgeUnknown Forge = iota
	ForgeGitHub
	ForgeGitLab
	ForgeCodeberg
)

// knownForges is the ordered set of forges gg models, host and all. Order is
// the probe/preference order for bare shorthand (github first).
var knownForges = []struct {
	forge Forge
	host  string
	name  string
}{
	{ForgeGitHub, "github.com", "github"},
	{ForgeGitLab, "gitlab.com", "gitlab"},
	{ForgeCodeberg, "codeberg.org", "codeberg"},
}

// KnownForges returns the modelled forges in preference order.
func KnownForges() []Forge {
	out := make([]Forge, len(knownForges))
	for i, f := range knownForges {
		out[i] = f.forge
	}
	return out
}

// Host returns the canonical hostname for a known forge, or "" for unknown.
func (f Forge) Host() string {
	for _, e := range knownForges {
		if e.forge == f {
			return e.host
		}
	}
	return ""
}

// String returns a short lowercase name ("github", "gitlab", "codeberg",
// "unknown") -- used in user-facing messages and picker labels.
func (f Forge) String() string {
	for _, e := range knownForges {
		if e.forge == f {
			return e.name
		}
	}
	return "unknown"
}

// ForgeFromHost maps a hostname to a modelled forge. The host is lowercased and
// a leading "www." is stripped first, so "WWW.GitHub.com" resolves to GitHub.
// An unrecognised (or empty) host yields ForgeUnknown.
func ForgeFromHost(host string) Forge {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	for _, e := range knownForges {
		if e.host == host {
			return e.forge
		}
	}
	return ForgeUnknown
}

// RepoRef is a parsed repo identity: which forge (if recognised), the raw host
// as written, and the user/org + repo. Host is "" for a bare "user/repo"
// shorthand, non-empty (but possibly unmodelled) for anything with a host.
type RepoRef struct {
	Forge Forge
	Host  string // canonical/raw host; "" for bare shorthand
	User  string
	Repo  string
}

// Shorthand reports whether the ref came from a bare "user/repo" (no host), so
// the clone flow knows to probe candidate forges rather than clone a fixed url.
func (r RepoRef) Shorthand() bool { return r.Host == "" && r.Forge == ForgeUnknown }

// URL returns the https clone url for this ref on its own forge/host.
func (r RepoRef) URL() string { return r.URLOn(r.Forge) }

// URLOn returns the https clone url for this ref's user/repo on forge f. Used
// to build candidate urls when probing a bare shorthand across forges.
func (r RepoRef) URLOn(f Forge) string {
	host := f.Host()
	if host == "" {
		host = r.Host // unmodelled host: fall back to what was written
	}
	return "https://" + host + "/" + r.User + "/" + r.Repo
}

// ParseRepoRef normalises the repo spellings gg accepts into a RepoRef. It is
// the generalisation of ParseGitHubURL across all forges plus shorthand:
//
//	full urls:      https://github.com/USER/REPO(.git), git@github.com:USER/REPO,
//	                ssh://git@github.com/USER/REPO(.git), git://github.com/USER/REPO
//	host shorthand: github.com/USER/REPO, www.github.com/USER/REPO (scheme optional)
//	bare shorthand: USER/REPO (no host -> Forge/Host unset; caller probes)
//
// ok is false when it can't pull a user + repo out at all (a lone token, an
// absolute/deep local path, empty input). A recognised host sets Forge; an
// unmodelled host leaves Forge unknown but Host populated (caller can still
// clone the url verbatim, just can't name the remote by convention).
func ParseRepoRef(raw string) (RepoRef, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return RepoRef{}, false
	}
	if i := strings.Index(s, "://"); i != -1 { // drop scheme
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "@"); i != -1 { // drop userinfo (git@)
		s = s[i+1:]
	}

	var host, path string
	hasHost := true
	colon := strings.IndexByte(s, ':')
	slash := strings.IndexByte(s, '/')
	switch {
	case colon != -1 && (slash == -1 || colon < slash):
		// scp form host:path
		host, path = s[:colon], s[colon+1:]
	case slash != -1:
		first := s[:slash]
		if strings.Contains(first, ".") {
			host, path = first, s[slash+1:] // host/path url form
		} else {
			hasHost = false // bare user/repo shorthand
			path = s
		}
	default:
		return RepoRef{}, false // single token, no separators
	}

	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return RepoRef{}, false
	}
	// A bare shorthand must be exactly user/repo -- a deeper path (a local
	// dir like a/b/c) is not shorthand we can resolve.
	if !hasHost && len(parts) > 2 && parts[2] != "" {
		return RepoRef{}, false
	}

	ref := RepoRef{User: parts[0], Repo: parts[1]}
	if hasHost {
		ref.Forge = ForgeFromHost(host)
		if ref.Forge != ForgeUnknown {
			ref.Host = ref.Forge.Host() // canonicalise (drop www., lowercase)
		} else {
			ref.Host = strings.ToLower(host)
		}
	}
	return ref, true
}
