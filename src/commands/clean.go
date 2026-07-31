package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/lczyk/gitgum/internal/dirty"
	"github.com/lczyk/gitgum/src/filetree"
)

// CleanCommand handles discarding working tree changes and untracked files.
// What counts as dirty, and what discarding it destroys, lives in
// internal/dirty -- the stash prompt offers the same operation, and a rule
// implemented twice is a rule that drifts.
type CleanCommand struct {
	cmdIO
	Changes   *bool `long:"changes" description:"Discard staged and unstaged changes (default: true)"`
	Untracked *bool `long:"untracked" description:"Remove untracked files (default: true)"`
	Ignored   *bool `long:"ignored" description:"Remove ignored files (default: false)"`
	All       bool  `long:"all" description:"Enable all cleanup options"`
	Yes       bool  `short:"y" long:"yes" description:"Skip confirmation prompt"`
}

func (c *CleanCommand) Execute(args []string) error {
	r := c.repo()
	if err := r.CheckInRepo(); err != nil {
		return err
	}

	changes := c.Changes == nil || *c.Changes
	untracked := c.Untracked == nil || *c.Untracked
	ignored := c.Ignored != nil && *c.Ignored

	if c.All {
		changes = true
		untracked = true
		ignored = true
	}

	// --ignored implies --untracked
	if ignored {
		untracked = true
	}

	if !changes && !untracked {
		fmt.Fprintln(c.out(), "Nothing to clean (all options disabled)")
		return nil
	}

	opts := dirty.Options{Tracked: changes, Untracked: untracked, Ignored: ignored}
	plan, err := dirty.Scan(r)
	if err != nil {
		return err
	}

	if plan.Count(opts) == 0 {
		fmt.Fprintln(c.out(), "Nothing to clean (working tree is clean)")
		return nil
	}

	printPlan(c.out(), plan, opts)

	if !c.Yes {
		confirmed, err := c.sel().Confirm("Proceed with cleanup? This cannot be undone", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(c.out(), "Cleanup cancelled")
			return nil
		}
	}

	if err := plan.Discard(r, opts); err != nil {
		return err
	}

	fmt.Fprintln(c.out(), "Clean complete")
	return nil
}

// maxDiscardLines bounds the listing. Three groups capped at 20 apiece was the
// old worst case, so nothing that used to be visible stops being -- and with
// intermediate directories folded away, the same budget now covers more files
// than it did.
const maxDiscardLines = 60

// printPlan lists what is about to be destroyed as one tree, each file marked
// with the code it arrived with. The groups still differ in how surprising
// their loss is -- a tracked edit, a file you created and a build artefact are
// different sizes of mistake -- so the summary line names them; per-file, the
// code says which is which.
func printPlan(out io.Writer, plan dirty.Plan, opts dirty.Options) {
	fmt.Fprintf(out, "Files to be discarded (%d)%s\n", plan.Count(opts), planSummary(plan, opts))
	filetree.Tree(out, planItems(plan, opts), filetree.Opts{
		Dim:        dim,
		FoldChains: true,
		MaxLines:   maxDiscardLines,
	})

	// Ignored files are always scanned, so their survival can be stated rather
	// than left to be discovered.
	if !opts.Ignored && len(plan.Ignored) > 0 {
		fmt.Fprintf(out, "  (%d ignored file(s) left alone; --ignored includes them)\n", len(plan.Ignored))
	}
}

// planItems is the plan's selected groups as one list to draw, each path
// carrying the code it arrived with. Unselected groups are absent rather than
// merely uncounted: this listing is what the caller is about to act on.
func planItems(plan dirty.Plan, opts dirty.Options) []filetree.Item {
	var items []filetree.Item
	group := func(paths []string, selected bool) {
		if !selected {
			return
		}
		for _, path := range paths {
			items = append(items, leafItem(plan.Codes[path], path, nil))
		}
	}
	group(plan.Tracked, opts.Tracked)
	group(plan.Untracked, opts.Untracked)
	group(plan.Ignored, opts.Ignored)
	return items
}

// planSummary breaks the count down by group, and says nothing when there is
// only one group to break it into -- "(3): 3 untracked" tells you nothing the
// count didn't.
func planSummary(plan dirty.Plan, opts dirty.Options) string {
	var parts []string
	if n := len(plan.Tracked); opts.Tracked && n > 0 {
		noun := "changes"
		if n == 1 {
			noun = "change"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, noun))
	}
	if n := len(plan.Untracked); opts.Untracked && n > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", n))
	}
	if n := len(plan.Ignored); opts.Ignored && n > 0 {
		parts = append(parts, fmt.Sprintf("%d ignored", n))
	}
	if len(parts) < 2 {
		return ":"
	}
	return ": " + strings.Join(parts, ", ")
}
