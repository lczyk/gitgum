package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

var knownQuirks = map[string]bool{
	"normal-branches": true,
}

func parseQuirks() (map[string]bool, error) {
	raw := os.Getenv("GG_QUIRKS")
	if raw == "" {
		return nil, nil
	}
	active := make(map[string]bool)
	for tok := range strings.SplitSeq(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		neg := strings.HasPrefix(tok, "-")
		name := strings.TrimPrefix(tok, "-")
		if !knownQuirks[name] {
			return nil, fmt.Errorf("unknown quirk %q (known: %s)", name, strings.Join(knownQuirkNames(), ", "))
		}
		if neg {
			delete(active, name)
		} else {
			active[name] = true
		}
	}
	return active, nil
}

// knownQuirkNames lists the modelled quirks in a stable order, so the error
// text doesn't reshuffle between runs (map iteration is randomised).
func knownQuirkNames() []string {
	out := make([]string, 0, len(knownQuirks))
	for k := range knownQuirks {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var (
	quirkWarnMu   sync.Mutex
	quirkWarnSeen = map[string]bool{}
)

// warnBadQuirks reports a malformed GG_QUIRKS on stderr, at most once per
// distinct value. A bad spec disables every quirk, not just the misspelled
// one, so staying silent turns a typo into "the flag I set does nothing" with
// no way to tell why.
func warnBadQuirks(raw string, err error) {
	quirkWarnMu.Lock()
	defer quirkWarnMu.Unlock()
	if quirkWarnSeen[raw] {
		return
	}
	quirkWarnSeen[raw] = true
	fmt.Fprintf(os.Stderr, "warning: ignoring GG_QUIRKS=%q: %v\n", raw, err)
}

func quirkEnabled(name string) bool {
	q, err := parseQuirks()
	if err != nil {
		warnBadQuirks(os.Getenv("GG_QUIRKS"), err)
		return false
	}
	return q[name]
}
