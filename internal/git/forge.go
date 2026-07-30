package git

import (
	"strconv"
	"strings"
)

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

// forgeAliases maps short spellings to a modelled forge, on top of the
// canonical names in knownForges. Canonical names stay the single source for
// display (String()); these are input-only conveniences.
var forgeAliases = map[string]Forge{
	"gh": ForgeGitHub,
	"gl": ForgeGitLab,
	"cb": ForgeCodeberg,
}

// ForgeFromName maps a forge name to a modelled forge: the canonical name
// ("github", "gitlab", "codeberg") or a short alias ("gh", "gl", "cb").
// Case-insensitive; an unrecognised (or empty) name yields ForgeUnknown. This
// is what resolves the "github/user/repo" (or "gh/user/repo") shorthand, where
// the leading token is a forge *name* rather than a host.
func ForgeFromName(name string) Forge {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, e := range knownForges {
		if e.name == name {
			return e.forge
		}
	}
	if f, ok := forgeAliases[name]; ok {
		return f
	}
	return ForgeUnknown
}

// looksLikeHost reports whether s is plausibly a hostname (as opposed to a
// path segment or a bare user name). A host has a dot-separated label and
// neither a leading nor trailing dot -- which rules out "." / ".." and the
// "./" / "../" prefixes of a local path, so those fall through to shorthand
// parsing (and get rejected as non-user/repo) rather than masquerading as a
// host and producing a bogus "https://../..." url.
func looksLikeHost(s string) bool {
	return strings.Contains(s, ".") &&
		!strings.HasPrefix(s, ".") && !strings.HasSuffix(s, ".")
}

// forgeNameHost reports the canonical host for a "forge/user/repo" shorthand:
// name must be a known forge NAME (not a host) and rest must be exactly
// "user/repo". Anything else yields "" so the caller falls back to bare
// "user/repo" shorthand -- this keeps "github/x" (a user literally named
// "github") and deeper local paths from being misread as a forge prefix.
func forgeNameHost(name, rest string) string {
	f := ForgeFromName(name)
	if f == ForgeUnknown {
		return ""
	}
	tail := strings.TrimSuffix(strings.Trim(rest, "/"), ".git")
	if p := strings.Split(tail, "/"); len(p) == 2 && p[0] != "" && p[1] != "" {
		return f.Host()
	}
	return ""
}

// RepoRef is a parsed repo identity: which forge (if recognised), the raw host
// as written, and the project path. Host is "" for a bare "user/repo"
// shorthand, non-empty (but possibly unmodelled) for anything with a host.
//
// Path is the whole project path. That is "user/repo" on every forge gg models
// except gitlab, which nests projects under subgroups ("group/team/project").
// Owner and Repo are positions within Path rather than stored alongside it, so
// they cannot drift from the path urls are built from.
type RepoRef struct {
	Forge Forge
	Host  string // canonical/raw host; "" for bare shorthand
	Path  string // full project path, e.g. "canonical/rockcraft"
}

// Owner is the first path segment: the user, org, or top-level group. This is
// what gg names a remote after.
func (r RepoRef) Owner() string {
	owner, _, _ := strings.Cut(r.Path, "/")
	return owner
}

// Repo is the last path segment: the project itself, which names the clone dir.
func (r RepoRef) Repo() string {
	if i := strings.LastIndex(r.Path, "/"); i >= 0 {
		return r.Path[i+1:]
	}
	return r.Path
}

// WithOwner returns the ref rehomed under a different owner, keeping only the
// project name. Subgroups are deliberately dropped: a fork lands in its new
// owner's namespace directly, not at the depth the upstream sat at.
func (r RepoRef) WithOwner(owner string) RepoRef {
	r.Path = owner + "/" + r.Repo()
	return r
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
	return "https://" + host + "/" + r.Path
}

// RouteKind classifies what a forge url pointed at inside a repo.
type RouteKind int

const (
	RouteNone   RouteKind = iota // the url named a repo and nothing more
	RoutePR                      // a pull request / merge request
	RouteTree                    // a branch or tag, possibly with a path below it
	RouteCommit                  // a single commit
	RouteOther                   // a route gg doesn't model (issues, releases, ...)
)

// Route is the part of a forge url that follows the project path.
type Route struct {
	Kind   RouteKind
	Number int    // PR/MR number; only meaningful for RoutePR
	Ref    string // branch/tag for RouteTree, sha for RouteCommit; unresolved as written
	Raw    string // the route segments verbatim
}

// ForgeURL is a fully parsed forge url: which repo, and what within it.
type ForgeURL struct {
	Ref   RepoRef
	Route Route
}

// splitProject divides a url path into the project path and whatever route
// follows it. The division is structural rather than a list of route names,
// because the shape of a project path is a fact about each forge:
//
//   - github and gitea (codeberg) host every project at exactly two segments,
//     so a third segment is a route whether or not gg recognises the name.
//   - gitlab nests projects under subgroups to arbitrary depth and marks the
//     boundary itself with "/-/", which is the only reliable way to tell a
//     subgroup from a route there.
//   - an unmodelled host has no routing scheme gg may assume, so the whole
//     path is the project.
func splitProject(f Forge, path string) (project, route string) {
	switch f {
	case ForgeGitLab:
		if before, after, ok := strings.Cut(path, "/-/"); ok {
			return before, after
		}
		return path, ""
	case ForgeUnknown:
		return path, ""
	default:
		owner, rest, ok := strings.Cut(path, "/")
		if !ok {
			return path, ""
		}
		repo, route, ok := strings.Cut(rest, "/")
		if !ok {
			return path, ""
		}
		return owner + "/" + repo, route
	}
}

// classifyRoute interprets route segments against a forge's url vocabulary.
// The vocabularies are small and only cover what gg acts on; anything else is
// RouteOther, which still resolves to "clone this repo" because the project
// path was already separated structurally.
func classifyRoute(f Forge, route string) Route {
	if route == "" {
		return Route{}
	}
	out := Route{Kind: RouteOther, Raw: route}
	head, rest, _ := strings.Cut(route, "/")

	switch {
	case f == ForgeGitHub && head == "pull",
		f == ForgeGitLab && head == "merge_requests",
		f == ForgeCodeberg && head == "pulls":
		if n, err := strconv.Atoi(firstSegment(rest)); err == nil && n > 0 {
			out.Kind, out.Number = RoutePR, n
		}
	case head == "tree", head == "blob":
		// the ref may itself contain slashes ("tree/feat/x"), and a file path
		// may follow it, so the tail stays unresolved until it can be matched
		// against the remote's actual refs.
		out.Kind, out.Ref = RouteTree, rest
	case head == "commit":
		out.Kind, out.Ref = RouteCommit, firstSegment(rest)
	case f == ForgeCodeberg && head == "src":
		// gitea spells these src/branch/<ref> and src/commit/<sha>
		kind, tail, _ := strings.Cut(rest, "/")
		switch kind {
		case "branch", "tag":
			out.Kind, out.Ref = RouteTree, tail
		case "commit":
			out.Kind, out.Ref = RouteCommit, firstSegment(tail)
		}
	}
	return out
}

func firstSegment(s string) string {
	seg, _, _ := strings.Cut(s, "/")
	return seg
}

// ParseRepoRef normalises the repo spellings gg accepts into a RepoRef,
// discarding any route the url carried. Callers that need the route (clone)
// use ParseForgeURL instead.
func ParseRepoRef(raw string) (RepoRef, bool) {
	fu, ok := ParseForgeURL(raw)
	return fu.Ref, ok
}

// ParseForgeURL normalises the repo spellings gg accepts into a RepoRef plus
// the route the url pointed at. It is the generalisation of ParseGitHubURL
// across all forges plus shorthand:
//
//	full urls:       https://github.com/USER/REPO(.git), git@github.com:USER/REPO,
//	                 ssh://git@github.com/USER/REPO(.git), git://github.com/USER/REPO
//	host shorthand:  github.com/USER/REPO, www.github.com/USER/REPO (scheme optional)
//	forge shorthand: github/USER/REPO (leading forge NAME, not host -> Forge set)
//	bare shorthand:  USER/REPO (no host -> Forge/Host unset; caller probes)
//
// ok is false when it can't pull a user + repo out at all (a lone token, an
// absolute/deep local path, empty input). A recognised host sets Forge; an
// unmodelled host leaves Forge unknown but Host populated (caller can still
// clone the url verbatim, just can't name the remote by convention).
func ParseForgeURL(raw string) (ForgeURL, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ForgeURL{}, false
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
		if looksLikeHost(first) {
			host, path = first, s[slash+1:] // host/path url form
		} else if h := forgeNameHost(first, s[slash+1:]); h != "" {
			host, path = h, s[slash+1:] // forge-name shorthand: github/user/repo
		} else {
			hasHost = false // bare user/repo shorthand
			path = s
		}
	default:
		return ForgeURL{}, false // single token, no separators
	}

	// The forge has to be known before the path can be split, because where a
	// project path ends is a per-forge fact (see splitProject).
	ref := RepoRef{}
	if hasHost {
		ref.Forge = ForgeFromHost(host)
		if ref.Forge != ForgeUnknown {
			ref.Host = ref.Forge.Host() // canonicalise (drop www., lowercase)
		} else {
			ref.Host = strings.ToLower(host)
		}
	}

	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	project, route := splitProject(ref.Forge, path)
	project = strings.TrimSuffix(project, ".git")

	segs := strings.Split(project, "/")
	if len(segs) < 2 {
		return ForgeURL{}, false
	}
	for _, seg := range segs {
		if seg == "" {
			return ForgeURL{}, false
		}
	}
	// A bare shorthand must be exactly user/repo -- a deeper path (a local
	// dir like a/b/c) is not shorthand we can resolve.
	if !hasHost && len(segs) > 2 {
		return ForgeURL{}, false
	}
	ref.Path = project
	return ForgeURL{Ref: ref, Route: classifyRoute(ref.Forge, route)}, true
}
