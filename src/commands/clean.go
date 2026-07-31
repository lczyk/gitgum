package commands

import (
	"fmt"
	"io"

	"github.com/lczyk/gitgum/internal/dirty"
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

// maxDisplayPerGroup bounds each listed group. The count in the header is
// always the true one -- it is the number that decides whether you say yes.
const maxDisplayPerGroup = 20

// printPlan lists what is about to be destroyed, one section per group. The
// groups are kept apart because losing a tracked edit, an untracked file and
// an ignored build artefact are different sizes of mistake, and a flat list
// makes them look the same.
func printPlan(out io.Writer, plan dirty.Plan, opts dirty.Options) {
	fmt.Fprintf(out, "Files to be discarded (%d):\n", plan.Count(opts))
	section := func(label string, paths []string, selected bool) {
		if !selected || len(paths) == 0 {
			return
		}
		fmt.Fprintf(out, "  %s (%d):\n", label, len(paths))
		for i, file := range paths {
			if i >= maxDisplayPerGroup {
				fmt.Fprintf(out, "    ... and %d more\n", len(paths)-maxDisplayPerGroup)
				break
			}
			fmt.Fprintf(out, "    %s\n", file)
		}
	}
	section("changes", plan.Tracked, opts.Tracked)
	section("untracked", plan.Untracked, opts.Untracked)
	section("ignored", plan.Ignored, opts.Ignored)

	// Ignored files are always scanned, so their survival can be stated rather
	// than left to be discovered.
	if !opts.Ignored && len(plan.Ignored) > 0 {
		fmt.Fprintf(out, "  (%d ignored file(s) left alone; --ignored includes them)\n", len(plan.Ignored))
	}
}
