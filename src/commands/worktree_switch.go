package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lczyk/gitgum/internal/doctor"
	"github.com/lczyk/gitgum/internal/git"
)

// WorktreeSwitchCommand prints the path of a worktree for the shell to cd
// into. A process cannot change its parent's directory, so the command's whole
// output contract is one path on stdout; the wrapper function emitted by
// `gg completion` does the cd (see src/completions). What is being left and
// entered goes to stderr, which the wrapper leaves alone.
type WorktreeSwitchCommand struct {
	cmdIO
	Args struct {
		N int `positional-arg-name:"N" description:"Worktree number (1 is <repo>, N is <repo>-N); omit to cycle to the next"`
	} `positional-args:"yes"`
}

// WorktreeSwitchBackCommand is WorktreeSwitchCommand's cycle run the other way.
type WorktreeSwitchBackCommand struct {
	cmdIO
}

// numberedWorktree is a worktree that follows doctor's <repo> / <repo>-N
// naming, with the N it answers to.
type numberedWorktree struct {
	Index int
	Path  string
}

var errNoWorktrees = errors.New("no worktrees follow the <repo> / <repo>-N naming; nothing to switch to")

// numberWorktrees keeps the worktrees that sit beside the main one and follow
// the <repo> / <repo>-N naming, sorted by N. Bare and prunable entries have no
// directory to cd into. When two dirs claim one N ("foo-2" and "foo-02") the
// first git lists wins and the other is named in warnings.
func numberWorktrees(wts []git.Worktree, repoName string) (out []numberedWorktree, warnings []string) {
	if len(wts) == 0 {
		return nil, nil
	}
	mainParent := filepath.Dir(filepath.Clean(wts[0].Path))
	taken := map[int]string{}
	for _, wt := range wts {
		if wt.Bare || wt.Prunable {
			continue
		}
		clean := filepath.Clean(wt.Path)
		if filepath.Dir(clean) != mainParent {
			continue
		}
		n, ok := doctor.RepoDirIndex(filepath.Base(clean), repoName)
		if !ok {
			continue
		}
		if prev, dup := taken[n]; dup {
			warnings = append(warnings, fmt.Sprintf("worktree %q also numbers as %d; keeping %q", clean, n, prev))
			continue
		}
		taken[n] = clean
		out = append(out, numberedWorktree{Index: n, Path: clean})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, warnings
}

// pickWorktree chooses the worktree to land in. n > 0 asks for that number
// outright. n == 0 cycles by step, +1 or -1: the entry after or before the
// current one, wrapping. A current dir that is not a numbered worktree at all
// sits outside the cycle, so stepping forward enters at the first and stepping
// back at the last.
func pickWorktree(entries []numberedWorktree, current string, n, step int) (numberedWorktree, error) {
	if len(entries) == 0 {
		return numberedWorktree{}, errNoWorktrees
	}
	if n < 0 {
		return numberedWorktree{}, fmt.Errorf("worktree number must be positive, got %d", n)
	}
	if n > 0 {
		for _, e := range entries {
			if e.Index == n {
				return e, nil
			}
		}
		return numberedWorktree{}, fmt.Errorf("no worktree numbered %d; have %s", n, listIndices(entries))
	}
	cur := -1
	for i, e := range entries {
		if e.Path == current {
			cur = i
			break
		}
	}
	if cur < 0 {
		if step < 0 {
			return entries[len(entries)-1], nil
		}
		return entries[0], nil
	}
	if len(entries) == 1 {
		return numberedWorktree{}, fmt.Errorf("only one numbered worktree (%s); nothing to cycle to", entries[0].Path)
	}
	return entries[(cur+step+len(entries))%len(entries)], nil
}

func listIndices(entries []numberedWorktree) string {
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = fmt.Sprint(e.Index)
	}
	return strings.Join(parts, ", ")
}

// canonicalPath resolves symlinks so a path git reports and a path the shell
// sits in compare equal (/tmp vs /private/tmp on macos). A path that cannot
// be resolved is compared as given.
func canonicalPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// repoDirName is the name the worktree dirs are expected to carry: the forge
// repo name when the remotes agree on one (doctor's rule), else the main
// worktree's own basename, so a repo with no forge remote still numbers.
func repoDirName(remoteURLs []string, mainPath string) string {
	var urls []string
	for _, entry := range remoteURLs {
		if _, url, ok := strings.Cut(entry, " "); ok {
			urls = append(urls, url)
		}
	}
	if names := doctor.ForgeRepoNames(urls); len(names) == 1 {
		return names[0]
	}
	return filepath.Base(filepath.Clean(mainPath))
}

// worktreeHeadRows is what `gg status head` prints from inside r.
func worktreeHeadRows(r git.Repo, color bool) ([]string, error) {
	branch, locals, err := r.HeadLine()
	if err != nil {
		return nil, fmt.Errorf("getting head of %s: %w", r.Dir, err)
	}
	return headRows(r, branch, locals, color), nil
}

func (w *WorktreeSwitchCommand) Execute(args []string) error {
	return switchWorktree(&w.cmdIO, w.Args.N, 1)
}

func (w *WorktreeSwitchBackCommand) Execute(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("worktree-switch- takes no arguments, got %d", len(args))
	}
	return switchWorktree(&w.cmdIO, 0, -1)
}

func switchWorktree(w *cmdIO, n, step int) error {
	r := w.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	var (
		wts      []git.Worktree
		toplevel string
		remotes  []string
	)
	if err := runConcurrent(
		func() (err error) {
			wts, err = r.Worktrees()
			if err != nil {
				return fmt.Errorf("listing worktrees: %w", err)
			}
			return nil
		},
		func() (err error) {
			toplevel, _, err = r.Run("rev-parse", "--show-toplevel")
			if err != nil {
				return fmt.Errorf("finding worktree root: %w", err)
			}
			return nil
		},
		func() (err error) {
			remotes, err = r.RemoteURLs()
			return err
		},
	); err != nil {
		return err
	}
	if len(wts) == 0 {
		return errNoWorktrees
	}
	for i := range wts {
		wts[i].Path = canonicalPath(wts[i].Path)
	}

	entries, warnings := numberWorktrees(wts, repoDirName(remotes, wts[0].Path))
	for _, warning := range warnings {
		fmt.Fprintln(w.err(), "warning:", warning)
	}

	current := canonicalPath(strings.TrimSpace(toplevel))
	target, err := pickWorktree(entries, current, n, step)
	if err != nil {
		return err
	}

	// The rows are a courtesy; the switch does not hang on them.
	color := colorEnabledOn(os.Stderr)
	var from, to []string
	if err := runConcurrent(
		func() (err error) {
			from, err = worktreeHeadRows(git.Repo{Dir: current}, color)
			return err
		},
		func() (err error) {
			to, err = worktreeHeadRows(git.Repo{Dir: target.Path}, color)
			return err
		},
	); err != nil {
		fmt.Fprintln(w.err(), "warning:", err)
	} else {
		for _, row := range append(from, to...) {
			fmt.Fprintln(w.err(), row)
		}
	}

	fmt.Fprintln(w.out(), target.Path)
	return nil
}
