package commands

import "sync"

// runConcurrent runs each fn in its own goroutine, waits for all to finish, and
// returns the first non-nil error in fn order (later errors are dropped). Every
// fn always runs to completion -- there's no early cancel -- so it's meant for a
// handful of quick, independent operations, not long or abortable ones.
//
// The branch pickers (branch/switch/delete) each front-load a few read-only git
// queries the picker can't open without: current branch, tracking remote,
// remotes, working-tree status. Those are independent subprocess spawns, so
// overlapping them shrinks the pre-picker delay from sum(reads) to max(reads) --
// noticeable on a large repo where `git status` alone runs into 100s of ms.
//
// Ordering the error by fn index lets callers put the query whose failure gives
// the best message first (e.g. delete's empty-repo guard), so it wins over a
// cryptic error from a sibling that failed for the same underlying reason.
func runConcurrent(fns ...func() error) error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	wg.Add(len(fns))
	for i, fn := range fns {
		go func() {
			defer wg.Done()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
